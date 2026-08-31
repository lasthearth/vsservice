---
Type: grilling
Status: resolved
Blocked by: 07, 08
---

# Kit-storage: контракт коллекции `kits` (Mongo-direct, два писателя)

## Question

Зафиксировать канонический контракт коллекции `kits` — единый источник кит-определений, из которого mail-домен разворачивает кит в `items`-вложения at-compose (тикет 10). Зеркало DailyRewards: **игра — владелец контента, Mongo-direct**, vsservice — читатель + писатель метаданных. Два писателя по разведённым полям, коллизии нет.

Решить:
- **bson-схема `kits`**: `kit_id`/`code` (ключ upsert, зеркало `savekit` role.Code), `title` (метаданные, пишет сайт), `items []KitItemDoc` (контент, пишет игра), `created_at`/`updated_at`, `updated_by`. `KitItemDoc`: `game_code` (namespace), `type` (item/block — 01/05), `quantity`, `attr_snapshot` (base64, формат из 07), `image_url` (CDN, из 08). Опираться на факты 07 (поля снимка) + 08 (`image_url`).
- **Разведение писателей**: захват (игра) делает `upsert by kit_id` с `$set` ТОЛЬКО контент-полей (`items`, `updated_at`); vsservice-rename делает `$set` ТОЛЬКО `title`/метаданных. Как гарантировать, что перезахват не сотрёт правленое сайтом `title` и наоборот — раздельные `$set`-пути, или title живёт вне items-документа. Точная форма upsert (без дублей — ключ `kit_id` unique).
- **vsservice read-модель**: Go читает `kits` (модель+repo+интерфейс в service-пакете по CLAUDE.md). Нужен ли отдельный `kit-def` домен `internal/kitdef/` или это подпапка mail/donate. Мутации через методы модели (конвенция).
- **vsservice writer метаданных**: RPC `RenameKit(kit_id, title)` (+ картинки предметов правятся? или image_url только из захвата 08 — вероятно только захват, сайт правит лишь title). Scope — админский (`kit:edit`?), роут `/v1/kits/{id}:rename`. Proto `proto/kit/v1` (старый internal/kit deprecated — новое proto-имя? `kitdef/v1`?).
- **Кросс-язык сериализация**: C#-MongoDB-драйвер пишет документ, Go-mongox читает. Согласовать имена bson-полей, формат `attr_snapshot` (base64-строка — обе стороны видят строку, не парсят на Go). Опереться на 07.
- **Список китов**: нужен ли `ListKits` RPC для сайта (выбрать кит для broadcast в админ-compose 04) — вероятно да.

Резолюция фиксирует: bson-схему `kits`, upsert-контракт захвата, разведение двух писателей, vsservice read-модель + proto/RPC метаданных, кросс-язык согласование. Разблокирует тикет 08 (куда пишет захват), тикет 10 (что читает разворот), связь с админ-compose 04.

## Answer

Зафиксирован канонический контракт коллекции `kits` — единый источник кит-определений. Зеркало DailyRewards: **игра — владелец контента (Mongo-direct), vsservice — читатель + писатель метаданных**. Два писателя по непересекающимся полям, коллизий нет.

### bson-схема `kits` (snake_case, заморожена — контракт кросс-язык C#↔Go)

```
kits {
  _id           ObjectId     // mongox, НЕ кросс-системный ключ
  code          string       // ключ, unique-индекс; пишет захват ($setOnInsert)
  title         string       // отображаемое название, метаданные сайта; пишет rename
  items         []KitItemDoc // контент; пишет захват
  created_at    time         // $setOnInsert (захват)
  updated_at    time         // оба писателя
}
KitItemDoc {
  game_code     string  // ПОЛНЫЙ namespaced Collectible.Code.ToString() (07); один и тот же код — источник image_url и ключ восстановления стека
  type          string  // "item" | "block" (01/05) — НЕ угадывается, едет явно
  quantity      int
  attr_snapshot string  // base64 бинарного TreeAttribute (07); "" когда Attributes.Count==0
  image_url     string  // <CDN_URL>/<bucket>/<uuidv7>.png (08)
}
```

- **`_id`** — обычный mongox ObjectId, но **никогда** не кросс-системный ключ (Q1).
- **`updated_by` выкинут** (YAGNI — читателю не нужен; добавить если всплывёт аудит).
- **`game_code`** = полный namespaced codename предмета (Q2). Один и тот же код — и ссылка на предмет для восстановления стека at-claim (`GetItem/GetBlock` по `type`), и то, из чего рендерится/именуется иконка (08). Отдельного «ID предмета» нет — `ItemStack.Id` (runtime-id) НЕ переносится (07).

### Ключ / идентичность (Q1)

- Ключ upsert = **`code`** (string, слаг из игры, напр. `starter`/`vip`), unique-индекс. Игра авторит имя (`OnKitCreate` арг / старый `savekit` `role.Code`); сайт имя не изобретает.
- `title` — отдельное поле отображаемого названия, правит только сайт (Q1).
- **Связь с donate**: `ShopItem.Code` (для `type=kit`) **равен** `kits.code`. Нового поля у `ShopItem` НЕ вводим, id-round-trip в игру НЕ нужен. (Детализация проброса `ShopItem.Code`→`kitExpander` — забота тикета 11.)

### Разведение двух писателей (Q3) — непересекающиеся `$set`, ключ `code`

- **Захват (C#, VintageAPI/Kit)** — upsert только контент:
  ```
  updateOne(
    { code },
    { $set:        { items, updated_at },
      $setOnInsert:{ code, title: "", created_at } },
    upsert: true)
  ```
  `title` только через `$setOnInsert` ⇒ перезахват никогда не перезаписывает сайтовый `title`. Unique-индекс на `code` ⇒ дублей нет.
- **Rename (Go, vsservice)** — `$set` только метаданные, **без upsert**:
  ```
  updateOne({ code }, { $set: { title, updated_at } })
  ```
  `NotFound` если кит ещё не захвачен (переименовывать несуществующий бессмысленно). **Go кит никогда не вставляет и не пишет контент.**

Кит рождается в игре (capture-first), `title` стартует пустым, сайт заполняет позже. Непересекающиеся наборы полей ⇒ ни один писатель не затирает другого.

### vsservice read-модель — новый домен `internal/kitdef/` (Q4)

- **Новый домен `internal/kitdef/`** по CLAUDE.md-layout: единолично владеет коллекцией `kits`. Model (`KitDef` + `KitItem`, мутация `title` через метод модели, конвенция), repo (mongo impl) + repo-интерфейс в service-пакете, sermapper/repomapper (goverter), fx.
- Go read-порт `GetKit(code) (*KitDef, error)` — его потребляет `kitExpander` mail (тикет 11): mail→kitdef, ацикл. Держит весь доступ к `kits` в одном домене, в стороне от legacy `internal/kit`, вне mail/donate.
- **Старый `internal/kit` (deprecated-ассайнмент) можно удалить целиком** (Q4) — но это отдельная задача вне маршрута карты (уже fog «Демонтаж старого internal/kit»); `kitdef` не наследует ничего от него, имя коллекции/proto новое.

### proto `kitdef/v1` + RPC + scope (Q5)

`proto/kitdef/v1/kitdef.proto`, сервис `KitDefService` (старый `kit/v1` остаётся для deprecated-ассайнмента):

- **`ListKits`** → `repeated KitDef{ code, title, created_at, updated_at, repeated KitItem{ game_code, type, quantity, image_url } }` — **`attr_snapshot` НЕ отдаётся по проводу** (только серверная сторона, зеркало DailyRewards `GetStateAsync`, где attributes stay server-side). Для выбора кита в admin broadcast «всем кит» (04/11). Роут `GET /v1/kitdefs`.
- **`RenameKit(code, title)`** → `POST /v1/kitdefs/{code}:rename`. `NotFound` если не захвачен; только структурная валидация (непустой `title`). Мутация через метод модели.
- Один админ-scope **`kit:edit`** на оба метода (оба — сайт-админ-surface). Раздельные scope позже, только если появится не-админ-чтение.

### кросс-язык bson-имена (Q6)

Канонический snake_case, заморожен: `code, title, items, game_code, type, quantity, attr_snapshot, image_url, created_at, updated_at`. C# `[BsonElement("…")]` (зеркало `DailyClaimItemDoc`) ↔ Go bson-теги совпадают байт-в-байт. `attr_snapshot` — непрозрачная base64-**строка** с обеих сторон, Go её не парсит (07, .NET `BinaryReader`-формат). C# захват владеет `_id`/`created_at`/`code`/`title:""` через `$setOnInsert`.

### temperature — стрипать, на границе выдачи (Q7, открытый из 07)

**Стрипать `temperature` при выдаче**, вместе с `transitionstate`, а НЕ при захвате. Снимок в `kits` остаётся честным захватом; единый чистильщик at-claim снимает оба subtree (вкл. вложенный `contents["0"]` контейнера). Обоснование: `temperature.temperatureLastUpdate` — календарно-относительный таймстемп; у кит-шаблона (захвачен раз, выдаётся месяцами) он протухает как и `transitionstate`, ломая cooldown-математику движка; кит из «горячих» предметов бессмыслен. Дёшево: `RemoveAttribute("temperature")`.

**⚠ Поправка вниз по карте:** это расширяет strip-набор из тикета 05 (сейчас только `transitionstate`) на `temperature` для **всех** выдач предметов at-claim, не только китов — консистентно и желательно. Реализуется в at-claim пути (05/11), прецеденты (`GiveKitItemsToPlayer` L384-392, `MongoDailyRewardBackend.RemoveTransitionState` L481-485) правятся добавлением `temperature` в тот же `RemoveAttribute`-блок.

**Разблокирует:** тикет 10 (куда/как пишет захват — `code`-upsert, форма `KitItemDoc`), тикет 11 (что читает `kitExpander` — `GetKit(code)` из `kitdef`, проброс `attr_snapshot`/`type`/`game_code`/`quantity` во вложения), связь с 04 (выбор кита через `ListKits`).

