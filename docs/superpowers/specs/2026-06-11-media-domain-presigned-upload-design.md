# Media-домен: presigned upload + хранение полного URL

**Дата:** 2026-06-11
**Статус:** утверждён, готов к планированию

## Проблема

Картинки (аватарки, иконки поселений, превью новостей, картинки donate-shop) сейчас обрабатываются двумя несогласованными путями:

1. **server-proxied** (avatar, settlement, news): клиент шлёт байты в gRPC → сервер конвертит в webp (аватар ещё ресайзит в X96/X48) → грузит в S3 → **собирает** URL через `Sprintf("%s/%s/%s", CdnUrl, bucket, file)`.
2. **presigned PUT** (donate через media-домен): клиент льёт прямо в S3, сервер байты не видит; donate валидирует, что `image_url` начинается с `CdnUrl/bucket/`.

Хочется, чтобы:
- юзер-API мог загрузить файл прямо в S3 и получить ссылку;
- модели в Mongo **хранили полный URL** (а не собирали его), что позволяет вставлять и внешние ссылки (imgur и т.п.);
- картинки (аватар, поселение) читались всеми посетителями — сейчас это решено публичным бакетом, остаётся.

## Решения (зафиксированы)

| Развилка | Выбор |
|---|---|
| Доверие к URL | Наш CDN + allowlist хостов (allowlist в конфиге) |
| Кто проверяет владение ресурсом | Media тупой; владение проверяет целевой домен |
| Обработка/варианты при presigned | Гибрид: presigned для обычных картинок + внешних; server-proxied остаётся там, где нужна обработка (аватар-варианты) |
| Объём итерации | Общий media + shared-валидация, donate-shop, settlement + news. **player-аватар не трогаем** |
| Лимит размера | Presigned **POST** + `content-length-range` (S3 режет по размеру/типу) |

## Архитектура

### Поток — наш CDN
```
client → MediaService.CreateUploadUrls(purpose, content_type?)
       → { post_url, fields, public_url }
       → multipart POST (fields + файл) на post_url   // S3 энфорсит размер/тип
       → <Domain>.Create(..., public_url)
       → mediaurl.Validate(public_url)                 // allowlist
       → хранит полный URL в Mongo
```

### Поток — внешняя ссылка
```
client → <Domain>.Create(..., "https://i.imgur.com/...")
       → mediaurl.Validate(url)   // imgur в allowlist
       → хранит полный URL
```

## Компоненты

### 1. Shared URL-валидатор — `internal/pkg/mediaurl`
Новый пакет.
- `Validator` с allowlist хостов.
- `Validate(rawURL string) error`: парсит URL; требует схему `https`; хост ∈ {хост из `CdnUrl`} ∪ allowlist. Возвращает sentinel-ошибку (не gRPC) — домены оборачивают в `status`.
- Провайдится мелким fx-модулем, инжектится в donate/settlement/news.

**Config** (`internal/pkg/config/config.go`): новое поле
```go
MediaAllowedHosts []string `envconfig:"MEDIA_ALLOWED_HOSTS"`
```
CDN-хост извлекается из существующего `CdnUrl`.

### 2. storage (pkg) — `internal/pkg/storage`
Новый метод (presigned POST):
```go
func (s *Storage) PresignedPostObject(
    ctx context.Context,
    bucket, object string,
    expiry time.Duration,
    maxSize int64,
    contentType string,
) (url string, fields map[string]string, err error)
```
Внутри: `minio.NewPostPolicy()` + `SetBucket`/`SetKey`/`SetExpires` + `SetContentLengthRange(0, maxSize)` + `SetContentType` (или `SetContentTypeStartsWith("image/")`, если тип не задан) → `client.PresignedPostPolicy(ctx, policy)`.

`PresignedPutURL` оставляем (может ещё использоваться).

### 3. media-сервис — proto + handler
**proto** (`proto/media/v1/media.proto`):
- `UploadPurpose` += `UPLOAD_PURPOSE_SETTLEMENT`, `UPLOAD_PURPOSE_NEWS` (DONATE_SHOP остаётся).
- `UploadTarget` меняется на POST-форму:
```proto
message UploadTarget {
  string post_url = 1;             // multipart POST endpoint
  map<string, string> fields = 2;  // поля формы (policy, signature, key, content-type…)
  string public_url = 3;           // финальный URL объекта
}
```
- `CreateUploadUrlsRequest` += опц. `string content_type` (default → `image/webp`; расширение объекта выводится из MIME).

**purpose-конфиг** (`internal/media/internal/service/app.go`) — вместо `purposeBuckets`:
```go
type purposeConfig struct {
    bucket       string
    maxSize      int64    // зашивается в POST-policy
    contentTypes []string // допустимые MIME (валидация content_type из запроса)
}
```
Значения (плейсхолдеры, финал — при импле):
- `DONATE_SHOP` → `donate-shop`, ~2 MiB
- `SETTLEMENT` → `settlements`, ~5 MiB
- `NEWS` → `news`, ~5 MiB

**handler** (`CreateUploadUrls`):
- count ∈ [1,20]; неизвестный purpose → `InvalidArgument`.
- если задан `content_type` — проверить, что входит в `contentTypes` purpose.
- **per-purpose scope**: map `purpose → требуемый scope`. `DONATE_SHOP`/`NEWS` → админский scope, `SETTLEMENT` → доступно любому аутентифицированному. Если у purpose есть scope — проверяем `claims.Scope` (через `interceptor.GetClaims`). Единый метод-scope `media:upload:create` снимается со `Scope()`; метод требует только аутентификацию.
  > Причина: метод-скоупер per-method, а нам нужно per-request (по purpose из тела). Поэтому проверка scope внутри хендлера.
- генерит `objectName = uuidv7 + ext`, зовёт `storage.PresignedPostObject`, собирает `public_url` (`CdnUrl/bucket/object`).

### 4. Bootstrap бакетов
Создание публичных бакетов для всех purposes переезжает в `internal/media/fx.go`: цикл по purpose-конфигу (`BucketExists` → `CreateBucket` → `MakeBucketPublic`). Дублирующее создание удаляется из `donate/fx.go`, `settlement/fx.go`, `news/fx.go`.

### 5. donate
`validateImageURL` (prefix-check в `internal/donate/internal/service/validate.go`) → вызов `mediaurl.Validate`. Внешние ссылки (imgur) проходят.

### 6. settlement
- proto (`proto/settlement/v1/settlement.proto`): `SubmitAttachment.data bytes` → `url string`.
- handler (`internal/settlement/internal/service/service.go`): валидирует каждый `url` через `mediaurl`, кладёт полный URL в `model.Attachment.Url`. Убирается webp-конверт + `UploadObject` + сборка URL.

### 7. news
- proto (`proto/news/v1/news.proto`): `CreateNewsRequest.preview bytes` → `string`.
- handler (`internal/news/internal/service/service.go`): валидирует через `mediaurl`, хранит полный URL. Убирается server-side upload + сборка.

### 8. Public read
Без изменений — бакеты публичные (`MakeBucketPublic`), внешние хосты публичны по природе.

## Обработка ошибок
- count вне [1,20] / неизвестный purpose / `content_type` не из списка → `InvalidArgument`.
- purpose требует scope, а его нет → `PermissionDenied`.
- URL не https / хост не в allowlist / мусор → `InvalidArgument` (домены оборачивают sentinel из `mediaurl`).
- превышение размера/типа при заливке → отклоняет сам S3 (POST-policy).

## Миграция данных
Не требуется. Уже сохранённые URL в settlement/news/donate — полные CDN-ссылки, проходят allowlist (CDN-хост разрешён). Аватар не трогаем.

## Breaking changes (proto)
- `settlement`: `SubmitAttachment.data` (bytes) → `url` (string).
- `news`: `CreateNewsRequest.preview` (bytes) → `string`.
- `media`: `UploadTarget` (`put_url` → `post_url` + `fields`).

Клиенты обновляются, `buf generate`.

## Тестирование
- `mediaurl.Validate`: cdn-хост / allowlist-хост / запрещённый хост / не-https / мусор.
- media handler: purpose→bucket+maxSize+scope, границы count, валидация content_type.
- donate/settlement/news: путь allowlist-валидации (наш CDN, внешний разрешённый, запрещённый).

## Вне объёма
- player-аватар (остаётся server-proxied с вариантами X96/X48).
- Асинхронный ресайз внешних/presigned-картинок.
- HEAD-проверка размера внешних ссылок.
