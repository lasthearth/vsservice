---
Type: research
Status: resolved
Blocked by:
---

# Заливка иконок предметов в MinIO из C# (VintageAPI-server)

## Question

Иконки предметов кита рендерятся на **клиенте** создающего кит (in-game), затем должны попасть в MinIO как PNG, чтобы сайт/лаунчер показывал кит с картинками. Игрок НЕ держит site-JWT vsservice (весь эффорт Mongo-direct), поэтому presigned-POST media-домена (`MediaService.CreateUploadUrls`, `ScopeAuthenticated`) недоступен из игры. Путь (тикет 08, Q5=A): клиент рендерит иконки → шлёт байты по игровому network-каналу на сервер-мод → **VintageAPI-server** кладёт PNG в MinIO своими creds (новый env, зеркало `VINTAGEAPI_MONGO_URL`).

Найти **факты** для контракта загрузки (тикет 08/09):

- MinIO/S3 SDK для C#/.NET: официальный `Minio` NuGet? Какой клиент (`MinioClient`), метод `PutObjectAsync` — сигнатура, endpoint/region/creds.
- Как vsservice-media конфигурит MinIO сейчас (`internal/pkg/storage`, env `MINIO_ENDPOINT/MINIO_ACCESS_KEY_FILE/MINIO_SECRET_KEY_FILE`): endpoint, TLS, bucket-политика (media-спека: бакеты публичные на чтение через `MakeBucketPublic`). Какой бакет под кит-иконки — новый (`kit-icons`?) или переиспользовать `donate-shop`.
- Публичный URL объекта: media собирает `CdnUrl/bucket/object`. Что писать в кит-определение как `image_url` предмета — тот же CDN-хост (mediaurl allowlist в vsservice его пропустит для сайта). Формат имени объекта (media: `uuidv7 + ext`).
- Клиентский рендер иконки предмета в PNG: есть ли в VS API готовый способ снять иконку `ItemStack` в изображение на клиенте (`capi.Render`/`GuiElementItemstackInfo`/`ItemstackTextureAtlas`), или это отдельная нетривиальная задача. (Если нетривиально — фактически влияет на объём тикета 08.)
- Bucket bootstrap: media создаёт бакеты в `internal/media/fx.go`. Кто создаёт `kit-icons` бакет — vsservice (media fx) или VintageAPI при старте.

Резолюция фиксирует: C#-SDK+метод загрузки, endpoint/creds-env, целевой бакет + политика, формат `image_url`, оценку сложности клиент-рендера иконки, кто bootstrap'ит бакет. Разблокирует тикет 08 (объём захвата) и 09 (форма `image_url` в контракте).

## Answer

Факты (полностью — `research/08-minio-csharp-upload-findings.md`, branch `research/minio-csharp-upload`):

- **C# SDK + upload:** NuGet `Minio` (7.0.0, net8+/netstd2.0). Клиент `new MinioClient().WithEndpoint("host:port").WithCredentials(ak,sk).WithSSL(bool).Build()`. Загрузка: `await minio.PutObjectAsync(new PutObjectArgs().WithBucket(b).WithObject(name).WithStreamData(stream).WithObjectSize(len).WithContentType("image/png"))`. (Версия рантайма определяется процессом VintageAPI, не игрой — если <net8, брать `Minio` 6.0.x, API совместим.)
- **Endpoint/creds env:** фактический код vsservice читает `MINIO_ENDPOINT`(host:port,без схемы)/`MINIO_ACCESS_KEY`/`MINIO_SECRET_KEY`/`MINIO_USE_SSL` (config.go:56-59; `*_FILE`-варианты из docs в коде отсутствуют). VintageAPI вводит зеркальные `VINTAGEAPI_MINIO_ENDPOINT/_ACCESS_KEY/_SECRET_KEY/_USE_SSL` на тот же MinIO-инстанс.
- **Целевой бакет + политика:** либо reuse `donate-shop` (уже существует, уже public-read — нулевая инфра-работа), либо новый `kit-icons`. Bootstrap бакетов — `internal/media/fx.go:26` OnStart hook (`BucketExists`→`CreateBucket`→`MakeBucketPublic`). Policy = anonymous `s3:GetObject` на `arn:aws:s3:::<bucket>/*`. Для нового бакета кто-то ОБЯЗАН применить эту policy (иначе не читается анонимно) — проще добавить `kit-icons` в slice media fx; VintageAPI тогда только пишет.
- **image_url формат:** `<CDN_URL>/<bucket>/<uuidv7>.png` (media: service.go:34,51; object name = `uuid.NewV7()`+ext). Allowlist (`internal/pkg/mediaurl/mediaurl.go`) проверяет **только host**: http(s) + host ∈ {host(CDN_URL)} ∪ `MEDIA_ALLOWED_HOSTS`. → любой бакет/объект под CDN-хостом vsservice проходит; VintageAPI должен генерить URL с тем же CDN-хостом.
- **Client icon render — нетривиально.** Готового "ItemStack → PNG-байты" в VS API нет. Строительные блоки (`vsapi Client/API/IRenderAPI.cs`): `RenderItemstackToGui(...)` (рисует стек в framebuffer, ortho stage/main thread), `RenderItemStackToAtlas(...)` (в atlas, thread-safe, callback→subid), `GrabScreenshot(w,h,scale,flip,withAlpha)`→`BitmapRef`, `BitmapRef.Pixels`/`.Save(filename)`, offscreen `CreateFrameBuffer`/`CurrentFrameBuffer`. Паттерн: offscreen framebuffer → `RenderItemstackToGui` → `GrabScreenshot(withAlpha:true)` → ARGB→PNG (через `Save` в temp-файл или SkiaSharp, движок уже юзает SkiaSharp/`SKColor`) → байты по game-network-каналу. **Средняя сложность, материально расширяет объём тикета 08** (render-в-текстуру + readback + PNG-энкод, не один вызов). `BitmapRef.Save` принимает filename → in-memory байты, вероятно, через temp-файл либо SkiaSharp; подтвердить реализацию через ilspycmd по движку при реализации.

Context: findings + branch `research/minio-csharp-upload`.
