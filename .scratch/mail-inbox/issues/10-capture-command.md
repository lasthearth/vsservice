---
Type: grilling
Status: resolved
Blocked by: 07, 08, 09
---

# Игровая команда захвата кита (VintageAPI/Kit: capture → MinIO + Mongo-upsert)

## Question

Спроектировать серверную сторону захвата: как игрок-админ снимает содержимое сундука/инвентаря в кит-определение, рендерит иконки, заливает в MinIO и апсертит в коллекцию `kits`. **Не выдача — создание/обновление шаблона** (map Notes, capturekit-разведение). Расширение существующего мода `Kit` `savekit` (`RiderProjects/Kit`), а НЕ gRPC (Q1=A, Mongo-direct).

Решить (опираясь на факты 07 сериализация, 08 MinIO+рендер, контракт 09):
- **Источник захвата**: сундук на который смотрит игрок vs инвентарь игрока (`savekit` сейчас берёт backpack/hotbar/armor игрока). Для кит-билдера вероятно целевой сундук (block entity container) — уточнить команду (`/capturekit <name>` глядя в сундук?).
- **Снимок предмета**: на каждый `ItemSlot` — `Code`, `EnumItemClass`→`type`, `StackSize`→`quantity`, **`attr_snapshot`** через сериализацию из 07 (дополнить текущий `ToKitItem` который attr теряет).
- **Рендер иконки** (клиент): server-команда триггерит клиентский рендер каждой иконки в PNG (способ из 08), клиент шлёт байты назад по network-каналу. Как связать рендер (клиент) с захватом (сервер) — request/response по каналу. Оценка сложности из 08.
- **Заливка PNG в MinIO** (сервер): VintageAPI кладёт байты своими creds (env из 08), получает CDN-`image_url` (формат 08).
- **Upsert в Mongo**: `kits`-коллекция, `$set` только контент-полей по `kit_id` (контракт 09), без дублей при перезахвате. Env — новый `VINTAGEAPI_MONGO_URL` уже есть (переиспользовать), MinIO-env новый.
- **Кто может** (privilege): `savekit` требует `commandplayer`. Оставить, или отдельный privilege.
- **Порядок**: рендер+upload иконок ДО или ПАРАЛЛЕЛЬНО с Mongo-upsert; частичный сбой (иконка не залилась) — кит без картинки или откат.

Резолюция фиксирует: команду захвата (источник, privilege), форму снимка предмета с attr, механизм клиент-рендер→сервер-upload, заливку MinIO, upsert-контракт, обработку частичного сбоя. Это VintageAPI+Kit-сторона, не vsservice.

## Answer

Захват — **не команда над сундуком**, а **утилити-блок для админов**: правый клик открывает vslibgui-GUI с большим инвентарём (64 слота) + текстовым полем под слаг + кнопкой «Сохранить». Админ раскладывает точный кит в блоке (накапливая между сессиями), задаёт `code`, жмёт Save. Иконки для веб-фронтенда — **в объёме** (веб-пикер китов показывает тумбнейлы; игровой UI иконки не использует — рендерит из `game_code`).

Архитектура — **зеркало DailyRewards** (Notes-инвариант): lhgui владеет блоком+GUI+транспортом, VintageAPI держит креды и пишет Mongo/MinIO через инъецируемый backend-сиам. VintageAPI↔vsservice = **только Mongo-direct + MinIO своими creds** (⚠-блок карты: НЕТ клиента к vsservice).

### Топология (R2-Q1 = A)

- **Блок `KitCaptureBlock` + `BlockEntityKitCapture`** — universal, регистрируется клиентом и сервером. Правый клик → проверка привилегии `controlserver` → открыть GUI (R2-Q5-privilege: **privilege на open, не команда**).
- **vslibgui-GUI** — клиентский (`Gui.dll`), 64-слотовый grid + textbox(`code`) + Save-кнопка.
- **Инвентарь блок-энтити** — server-authoritative `InventoryGeneric(64)`, персист в chunk-данных через `ToTreeAttributes`/`FromTreeAttributes` (переживает рестарт; кит собирается постепенно). Размер фикс — конфиг слотов YAGNI.
- **Бэкенд-сиам `IKitCaptureBackend`** (в lhgui, зеркало `IDailyRewardBackend`/`IMailBackend`) реализуется VintageAPI (`Secrets.MongoUrl` → `MongoClient` → `GetCollection("kits")` + MinIO), инъекция через `SetBackend`. Блок server-only быть не может (клиентский GUI-рендер) — сиам обязателен.

### Транспорт Save (R2-Q2 = B) — выделенный канал

Выделенный сетевой канал **`lhgui-kitcapture`** (паттерн DailyRewards: `RegisterChannel` + `SetMessageHandler`), НЕ пакеты блок-энтити. Save-пакет несёт контекст явно: `{ blockPos, code, KitIconPacket[]{ slotIndex, png[] } }` (позиция блока в пакете, т.к. канал блок-контекст не знает). Хендлер на сервере зовёт `IKitCaptureBackend.CaptureAsync(...)`. Обоснование выбора: привычный проекту паттерн, единообразие с остальными lhgui-фичами.

### Снимок предмета (R2-Q3-snapshot = как есть)

На каждый непустой `ItemSlot` — переиспользуем проверенный сниппет `NatsKitsSystem.OnKitCreate` (L496-503):
- `game_code` = `stack.Collectible.Code.ToString()` (полный namespaced, 07/09);
- `type` = `Collectible.ItemClass` → `"item"`/`"block"` (07: нести **явно**, не гадать — фикс бага `SerializedItem`/`ToKitItem`, которые ItemClass роняют);
- `quantity` = `slot.StackSize`;
- `attr_snapshot` = base64(`stack.Attributes.ToBytes()`), **пусто** при `Attributes.Count==0`.

**Две поправки из 07**: (1) чистый `ToBytes()` (`ToArray()`), НЕ `stream.GetBuffer()` (тянет хвост нулей); (2) **никогда** не персистить `ItemStack.Id` (runtime-id). **Никакого стрипа** `temperature`/`transitionstate` при захвате — снимок честный, стрип на границе выдачи (09-Q7/05).

### Снимок vs рендер (R2-Q5 = A) — сервер контент, клиент пиксели

- **Клиент** при Save рендерит каждый непустой слот в PNG (08: offscreen `FrameBufferRef` → `RenderItemstackToGui` → `GrabScreenshot(withAlpha:true)` → `BitmapRef` → ARGB→PNG; нетривиально, нет one-call API), шлёт `KitIconPacket[]{slotIndex, png}`.
- **Сервер** по своему **authoritative** инвентарю блок-энтити строит `KitItemDoc[]` (game_code/type/qty/attr), матчит PNG по `slotIndex`. Контент не зависит от клиентской синхронизации; рендер там, где единственно возможен (клиент).

### Upsert-контракт (09) — код-слаг

`code` из текстового поля (R2-Q4): обязателен, непустой, валидация слага (`^[a-z0-9-]+$`). `title` в игре **не редактируется** (сайт-owned, `RenameKit`). Upsert строго по контракту 09:
```
updateOne({ code },
  { $set:        { items, updated_at },
    $setOnInsert:{ code, title:"", created_at } },
  upsert:true)
```
`title` только через `$setOnInsert` ⇒ перезахват не трёт сайтовый title. Unique-индекс на `code` ⇒ дублей нет. Env: переиспользовать `VINTAGEAPI_MONGO_URL`; MinIO — новые `VINTAGEAPI_MINIO_*` (08).

### Порядок / частичный сбой (R2-Q6) — контент первым, иконки best-effort, без отката

1. **Upsert контента ПЕРВЫМ** (`$set:{items,updated_at}`) — то, что нужно почте на claim-time; кит сразу юзабелен.
2. **Потом** best-effort: upload каждой PNG в MinIO → `$set` `image_url` по слоту.

Обоснование: разворот кита (11) **никогда** не читает `image_url` — сбой иконки не блокирует существование/юзабельность кита. Провал иконки = `image_url=""` у предмета, чинится перезахватом или бэкфиллом. **Без отката** (дух DailyRewards: «краш теряет доставку, но не дублирует»).

### Бакет MinIO (R2-Q7) — новый `kit-icons`

Новый бакет `kit-icons`: bootstrap строкой в slice `internal/media/fx.go` vsservice (vsservice ставит public-read policy через `MakeBucketPublic`); VintageAPI только `PutObjectAsync` своими creds. `image_url` = `<CDN_host>/kit-icons/<uuidv7>.png`; VintageAPI знает CDN-хост как факт-конфиг (allowlist 08 проверяет только host → тот же CDN-хост проходит). Чистое разделение жизненного цикла кит-иконок от донат-медиа.

### Итоговая форма

- **Блок**: `KitCaptureBlock`+`BlockEntityKitCapture` (universal), open-privilege `controlserver`, `InventoryGeneric(64)` персист в chunk.
- **GUI**: vslibgui, 64-слот grid + textbox(`code`) + Save.
- **Канал**: `lhgui-kitcapture`, Save-пакет `{blockPos, code, KitIconPacket[]}`.
- **Сиам**: `IKitCaptureBackend` (lhgui), реализация VintageAPI (Mongo+MinIO), `SetBackend`.
- **Снимок**: сервер-authoritative, `game_code`+`type`+`quantity`+`attr_snapshot` (07, чистый `ToBytes`, без `Id`, без strip).
- **Иконки**: клиент рендерит PNG (08), сервер матчит по слоту, upload в `kit-icons`.
- **Запись**: upsert контента первым (09-контракт), иконки best-effort без отката.
