# Findings: заливка иконок предметов в MinIO из C# (VintageAPI-server)

Branch: `research/minio-csharp-upload`
Ticket: `.scratch/mail-inbox/issues/08-minio-csharp-upload-research.md`
Facts only. No design decisions.

## 1. C# / .NET MinIO SDK

- Официальный пакет: **`Minio`** на NuGet (owner `minio`, repo `minio/minio-dotnet`, Apache-2.0). Latest **7.0.0** (2025-11-05). Targets `net8.0` + `netstandard2.0` (also net5/6/7/9/10). Source: nuget.org/packages/Minio, github.com/minio/minio-dotnet README.
- **VS-релевантно:** мод грузится в игровой процесс, но upload происходит на **VintageAPI-server** (отдельный .NET-процесс), не в игровом клиенте — так что версия рантайма определяется VintageAPI, не игрой. Проверить TFM VintageAPI перед выбором 7.0.0 vs 6.0.x (7.0.0 требует net8+; если VintageAPI на более старом рантайме — брать 6.0.x, API совместим).

### Клиент — два стиля инициализации (README, master)

Fluent builder (стабильный, во всех 6.x/7.x):
```csharp
using Minio;
using Minio.DataModel.Args;

IMinioClient minio = new MinioClient()
    .WithEndpoint(endpoint)          // "host:port" БЕЗ схемы, напр. "minio.example.com:9000"
    .WithCredentials(accessKey, secretKey)
    .WithSSL(secure)                 // bool; true = HTTPS
    .Build();
```

### PutObjectAsync — сигнатура (args-объект, 6.x/7.x)
```csharp
var args = new PutObjectArgs()
    .WithBucket(bucketName)
    .WithObject(objectName)          // напр. "<uuidv7>.png"
    .WithStreamData(stream)          // Stream с PNG-байтами
    .WithObjectSize(stream.Length)   // long; размер потока
    .WithContentType("image/png");
await minio.PutObjectAsync(args).ConfigureAwait(false);
```
(Вариант с файлом: `.WithFileName(path)` вместо `.WithStreamData/.WithObjectSize`.)
Из in-memory PNG-байтов: `using var ms = new MemoryStream(pngBytes); .WithStreamData(ms).WithObjectSize(pngBytes.Length)`.

Region: по умолчанию не нужен для self-hosted MinIO (S3 path-style). Если требуется — `.WithRegion("us-east-1")` на билдере.

Bucket-ensure на старте (mirror логики Go-media): `BucketExistsArgs`→`MakeBucketArgs`, метод `minio.BucketExistsAsync` / `minio.MakeBucketAsync`.

## 2. Как vsservice конфигурит MinIO сейчас (контракт, который C# должен зеркалить)

`internal/pkg/config/config.go` (envconfig):
```
MINIO_ENDPOINT     -> MinioEndpoint  (host:port, без схемы)
MINIO_ACCESS_KEY   -> MinioAccessKey
MINIO_SECRET_KEY   -> MinioSecretKey
MINIO_USE_SSL      -> MinioUseSSL  (bool)
```
> Примечание: в CLAUDE.md/AGENTS.md упомянуты `MINIO_ACCESS_KEY_FILE`/`MINIO_SECRET_KEY_FILE` (file-based secrets), но **фактический код читает `MINIO_ACCESS_KEY` / `MINIO_SECRET_KEY` напрямую** (config.go:56-58). `*_FILE`-вариантов в коде нет. Docs устарели.

Клиент (`main.go:123-133`, `minio-go/v7`):
```go
minio.New(c.MinioEndpoint, &minio.Options{
    Creds:  credentials.NewStaticV4(c.MinioAccessKey, c.MinioSecretKey, ""),
    Secure: c.MinioUseSSL,
})
```
→ endpoint = `host:port` без схемы; static V4 creds; TLS по флагу.

**C#-маппинг:** `MinioEndpoint`→`.WithEndpoint(...)`, `MinioAccessKey`→ access, `MinioSecretKey`→ secret, `MinioUseSSL`→`.WithSSL(bool)`. Идентичная семантика.

**Env-shape для VintageAPI (новый, зеркало `VINTAGEAPI_MONGO_URL`):** ввести собственные `VINTAGEAPI_MINIO_ENDPOINT` / `VINTAGEAPI_MINIO_ACCESS_KEY` / `VINTAGEAPI_MINIO_SECRET_KEY` / `VINTAGEAPI_MINIO_USE_SSL`, указывающие на **тот же MinIO-инстанс**, что и vsservice (тот же endpoint/creds или отдельный service-account с write на целевой бакет). Значения по форме идентичны vsservice.

## 3. Bucket-политика (public-read)

`internal/pkg/storage/storage.go:17 MakeBucketPublic` ставит bucket policy — anonymous `s3:GetObject` на `arn:aws:s3:::<bucket>/*`:
```json
{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::<bucket>/*"]}}
```
Бакет создаётся через `MakeBucket` (`CreateBucket`, storage.go:38), затем эта политика.

## 4. Bucket bootstrap

`internal/media/fx.go:23-44` — fx `OnStart` hook в модуле media идемпотентно создаёт `["donate-shop","settlementsreq","news"]`: для каждого `BucketExists`→(если нет)`CreateBucket`+`MakeBucketPublic`.

**Целевой бакет под кит-иконки:** списка пока нет `kit-icons`. Два факта-варианта:
- **Reuse `donate-shop`** — уже существует, уже public-read, bootstrap на месте. Нулевая инфра-работа.
- **Новый `kit-icons`** — добавить строку в slice `internal/media/fx.go:26`; vsservice-media при старте создаст+сделает public. VintageAPI при старте может просто `BucketExistsAsync` (не обязан создавать, если полагается на vsservice bootstrap), либо создать сам через `MakeBucketAsync` для независимости от порядка старта.

Факт для тикета: **если новый бакет — кто-то ДОЛЖЕН вызвать `MakeBucketPublic`/эквивалент policy**, иначе объекты не будут читаться анонимно сайтом/лаунчером. Проще всего — добавить `kit-icons` в media fx slice (vsservice уже умеет ставить public policy); VintageAPI тогда только пишет объекты.

## 5. Публичный URL объекта + allowlist

Media строит public URL как `CdnBase/bucket/object` (`internal/media/internal/service/service.go:34,51`):
```
fmt.Sprintf("%s/%s/%s", strings.TrimRight(cfg.CdnUrl,"/"), bucket, objectName)
```
Object name = **uuidv7 + ext** (service.go:38-43; `uuid.NewV7()`), ext по content-type (`.png` для `image/png`, app.go:52-60).

**Формат `image_url` для кит-определения:**
```
<CDN_URL>/<bucket>/<uuidv7>.png
```
напр. `https://cdn.lasthearth.ru/kit-icons/018f....png` (или `.../donate-shop/...`).

**Allowlist-правило** (`internal/pkg/mediaurl/mediaurl.go`): `Validate` пропускает URL, если:
1. schema `http`/`https`,
2. непустой host,
3. host ∈ allowlist, где allowlist = `{host(CDN_URL)} ∪ MEDIA_ALLOWED_HOSTS`.
→ Проверяется **только host**, не путь/бакет. **Достаточно, чтобы `image_url` использовал тот же CDN-хост, что и `CDN_URL` vsservice** — тогда любой бакет/объект под этим хостом проходит. VintageAPI должен генерировать URL с host = host из `CDN_URL` vsservice (нужен как общий факт-конфиг: VintageAPI должен знать CDN-хост).

## 6. Client-side рендер иконки предмета в PNG — оценка сложности

**Нет готового one-call "ItemStack → PNG-байты".** Релевантные API (`vsapi` `Client/API/IRenderAPI.cs`, master):

- `RenderItemstackToGui(ItemSlot inSlot, x, y, z, size, color, [dt], shading, rotate, showStackSize)` (:673/:688) — рисует стек в **текущий framebuffer** в ortho/gui-режиме. Требует ortho render stage (main thread). Сам по себе НЕ даёт байты — только рисует.
- `RenderItemStackToAtlas(ItemStack stack, ITextureAtlasAPI atlas, int size, Action<int> onComplete, color, sepiaLevel, scale)` (:701) — рендерит в **переданный texture atlas**, thread-safe, callback возвращает texture subid. Дорого, кэшируется в атласе; извлечь чистые пиксели одного стека из общего атласа неудобно.
- `BitmapRef GrabScreenshot(int width, int height, bool scaleScreenshot, bool flip, bool withAlpha=false)` (:375) — читает **primary render framebuffer** в `BitmapRef`.
- `BitmapRef` (`Common/Texture/BitmapRef.cs`): имеет `int[] Pixels`, `void Save(string filename)` (:119) и `CropTo(int)`. `Save` пишет в файл (реализация закрыта в движке, но по формату .png — VS хранит текстуры как png).
- Framebuffer API: `CreateFrameBuffer(FramebufferAttrs)` (:851), `CurrentFrameBuffer{get;set;}` (:60), `RenderTextureIntoFrameBuffer` (:855), `DestroyFrameBuffer` (:857) — можно рендерить offscreen.

**Рабочий паттерн (нетривиальный, ~средняя сложность):**
1. На клиенте, внутри ortho render stage (main thread), создать/забиндить offscreen `FrameBufferRef` нужного размера (или использовать primary), очистить прозрачным.
2. `RenderItemstackToGui(slot, ...)` для каждой иконки.
3. `GrabScreenshot(w, h, false, flip, withAlpha:true)` → `BitmapRef` → `Pixels` (ARGB int[]) или `Save`.
4. Кодировать в PNG-байты (через `BitmapRef.Save` во временный файл + чтение, либо через SkiaSharp — движок уже использует `SkiaSharp`/`SKColor`, см. BitmapRef.cs) и слать по game-network-каналу на сервер-мод.

**Оценка:** это **отдельная нетривиальная задача**, не однострочник. Требует: работа в правильной render-stage фазе (main thread, ortho), корректный alpha (`withAlpha:true`), возможно offscreen-framebuffer чтобы не портить кадр, конверсия ARGB→PNG. Ни одного публичного API "стек → PNG-байты" нет — только строительные блоки. Материально влияет на объём тикета 08 (захват): закладывать полноценную реализацию рендер-в-текстуру + readback + PNG-энкод, не «дёрнуть один метод».

Открытый риск/факт для 08: `BitmapRef.Save` пишет в файл (сигнатура принимает filename), поэтому для in-memory PNG-байтов, вероятно, придётся либо через temp-файл, либо через SkiaSharp напрямую из `Pixels`/`GetPixelsTransformed`. Подтвердить, когда будет доступ к рантайму движка (ilspycmd по `Vintagestory.exe` для реализации `BitmapRef.Save`/конкретного подкласса).

## Сводка (факты)

| Вопрос | Факт |
|---|---|
| C# SDK | NuGet `Minio` 7.0.0 (net8+/netstd2.0), `IMinioClient` через `new MinioClient().WithEndpoint/WithCredentials/WithSSL().Build()` |
| Upload-метод | `PutObjectAsync(new PutObjectArgs().WithBucket().WithObject().WithStreamData(stream).WithObjectSize(len).WithContentType("image/png"))` |
| Endpoint/creds env | `MINIO_ENDPOINT`(host:port,без схемы)/`MINIO_ACCESS_KEY`/`MINIO_SECRET_KEY`/`MINIO_USE_SSL`; VintageAPI зеркалит как `VINTAGEAPI_MINIO_*` на тот же инстанс |
| Целевой бакет | reuse `donate-shop` (уже public) ИЛИ новый `kit-icons` (добавить в media fx slice + `MakeBucketPublic`) |
| Bucket policy | anonymous `s3:GetObject` на `arn:aws:s3:::<bucket>/*` (`MakeBucketPublic`) |
| Bucket bootstrap | `internal/media/fx.go` OnStart hook (vsservice-media); для нового бакета добавить строку туда |
| image_url формат | `<CDN_URL>/<bucket>/<uuidv7>.png` |
| Allowlist-правило | `mediaurl.Validate`: http(s) + host ∈ {host(CDN_URL)} ∪ MEDIA_ALLOWED_HOSTS. Только host важен → тот же CDN-хост проходит любой бакет |
| Client icon render | Нетривиально. Нет "ItemStack→PNG". Блоки: `RenderItemstackToGui`/`RenderItemStackToAtlas` + framebuffer + `GrabScreenshot`→`BitmapRef`→PNG. Средняя сложность, расширяет объём тикета 08 |
