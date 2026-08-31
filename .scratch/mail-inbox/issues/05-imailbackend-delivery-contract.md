---
Type: grilling
Status: resolved
Blocked by: 01
---

# Контракт доставки IMailBackend (VintageAPI)

## Question

Backend-сиам в lhgui `Mail/IMailBackend.cs` и его реализация в VintageAPI (чтение mail из общего Mongo, идемпотентная выдача в игру). Зеркало `IDailyRewardBackend`/`MongoDailyRewardBackend`.

Решить:
- Сигнатуры сиама: `Task<MailListPacket?> GetMailAsync(player)`, `Task<MailStatus> MarkReadAsync(player, id)`, `Task<MailStatus> ClaimAsync(player, id)` (+ ClaimAll?). null = скрыть секцию (fail-closed, как DailyRewards).
- Идемпотентность claim: запись строки `mail_claims` `(mail_id, player_id)` в `claimed` **до** выдачи (claimed-before-grant, тикет 01, Q6); статусы Granted/AlreadyClaimed/Expired/Revoked/InventoryHandled/Unavailable.
- Резолв игрока: `PlayerName`/`PlayerUID` → site `user_id` (существующий `PlayerIdentityResolver` в VintageAPI). Broadcast и targeted единообразны — оба через `mail_claims` per-player (тикет 01, Q6), спецслучая нет.
- Физическая выдача: items — `TryGiveItemstack`→`SpawnItemEntity`, снятие `transitionstate`, merge attr-снапшота (паттерн Mine/DailyRewards `ResolveStack`); coins — идемпотентный `$inc` в `donate_wallets` + аудит `donate_transactions`. **kit-ref убран** (тикет 01, Q1=B): бэкенд выдаёт только `items`+`coins`, кит уже развёрнут в item-вложения на стороне vsservice at-compose.
- Marshalling на main thread (`EnqueueMainThreadTask`), backend off-thread (`Task.Run`), fail-closed на Mongo-фолтах.
- Читает VintageAPI Mongo напрямую (как MongoDailyRewardBackend) — да, подтвердить коллекции/индексы.

Разблокирует UI-тикет 06.

Резолюция: сигнатуры сиама и статус-enum, стратегия резолва игрока+broadcast, схема идемпотентного claim, маппинг каждого типа вложения на игровую выдачу.

## Answer

Контракт `IMailBackend` — точное зеркало `IDailyRewardBackend` (сиам в lhgui, Mongo-реализация в VintageAPI, `SetBackend`, off-tick `Task.Run` + `EnqueueMainThreadTask`, fail-closed hide). Заземлено на прочитанный код: `MongoDailyRewardBackend`, `PlayerIdentityResolver`, `DailyRewardModSystem`, донат `AddCoinsToWallet` (vsservice).

### Сиам (lhgui `Mail/IMailBackend.cs`) — 3 метода

```csharp
public interface IMailBackend {
    Task<MailListPacket?> GetMailAsync(IServerPlayer player);            // null = скрыть секцию (fail-closed)
    Task<MailStatus>      MarkReadAsync(IServerPlayer player, string mailId);
    Task<MailStatus>      ClaimAsync(IServerPlayer player, string mailId);
}
```

`ClaimAll` **не вводим** (Q5): UI (06) циклит `ClaimAsync` по письмам, каждый идемпотентен и ревалидируется сервером. Держим сиам на 3 методах как DailyRewards. Агрегатный `ClaimAll` → **fog** (добавить только если 06 покажет реальную round-trip цену).

### enum `MailStatus` (Q3)

```csharp
public enum MailStatus {
    Granted        = 0,  // ClaimAsync: выдача прошла (инвентарь ИЛИ земля — снаружи неразличимо;
                         //             частично-нерезолвимые вложения залогированы — Q4)
    AlreadyClaimed = 1,  // ClaimAsync: строка mail_claims уже в claimed (DuplicateKey на insert-гейте)
    Expired        = 2,  // ClaimAsync: now >= expires_at, непроклеймлено → claim заблокирован
    Revoked        = 3,  // ClaimAsync: флаг revoked на документе, непроклеймлено → claim заблокирован
    Unavailable    = 4,  // ЛЮБОЙ метод: ResolveUserIdAsync == null ИЛИ Mongo-фолт. Fail-closed.
    Read           = 5,  // MarkReadAsync: успех (идемпотентно)
}
```

`InventoryHandled` **убран** (Q3): `TryGiveItemstack`→`SpawnItemEntity` тихий, оба = `Granted` (зеркало DailyRewards); вынос его наружу = UI-копирайт под не-событие.

Раскладка по методам:
- **`GetMailAsync`** → `MailListPacket?`, enum НЕ возвращает. `null` = скрыть секцию (Mongo-фолт / игрок не резолвится), зеркало `GetStateAsync`. Per-письмо стейт (`unread/read/claimed/expired/revoked`) — внутри пакета, вычислен джоином (Q7); это отдельный per-mail state, не `MailStatus`.
- **`MarkReadAsync`** → `Read` (успех, идемпотентно) | `Unavailable` (не резолвится/фолт/письма нет/не адресовано игроку — не течём деталями чужих писем). `expired`/`revoked` письмо пометить read можно (серое, но открыто) — не ошибка.
- **`ClaimAsync`** → `Granted | AlreadyClaimed | Expired | Revoked | Unavailable`. **Порядок проверок**: резолв игрока (`Unavailable`) → письмо существует и адресовано (`Unavailable`) → флаг `revoked` (`Revoked`) → `expires_at` (`Expired`) → insert-гейт `mail_claims` (`DuplicateKey` ⇒ `AlreadyClaimed`) → выдача → `Granted`. Пустое письмо-уведомление, если долетит claim → `Granted` no-op (гейт всё равно пишет `claimed`).

Lang-ключи для 06: `mail-refuse-alreadyclaimed / -expired / -revoked / -unavailable` (зеркало `dailyrewards-refuse-*`).

### Резолв игрока + единообразие broadcast/targeted

`PlayerIdentityResolver.ResolveUserIdAsync(player)` **переиспользуется как есть** — ключ по `PlayerName` (case-sensitive, коммент в коде: один ник = один site-account даже при нескольких игровых копиях), читает `verification_requests` (`user_game_name` + `status in {verified,approved}`), возвращает site `user_id` или null (fail-closed). null ⇒ `Unavailable` (claim) / `null`-пакет (get). Broadcast и targeted **единообразны** — оба через `mail_claims` per-player, спецслучая нет (01, Q6).

### `GetMailAsync` — read-only джоин (Q7)

Чтение **не мутирует** `mail_claims` (избегаем fan-out записей на каждое открытие; broadcast-фан-аут перф = fog в карте). Эффективный per-player стейт = джоин `mail` (recipient `== user_id` ИЛИ `== "broadcast"`, не `revoked` для скрытия — либо показ серым) с существующими `mail_claims (mail_id, player_id)`:
- строки `mail_claims` нет → `unread` (broadcast, ещё не тронутое);
- есть → её `state` (`read`/`claimed`);
- `now >= expires_at` и не `claimed` → `expired` (лениво выводится при чтении, TTL не удаляет — 01, Q5a);
- флаг `revoked` на документе и не `claimed` → `revoked` (01, Q5b).

Строки пишут только `MarkReadAsync` (upsert `read`) и `ClaimAsync` (insert `claimed`). Зеркало DailyRewards: чтения read-only, терминальные стейты выводятся, claim отклоняется на сервере.

### Идемпотентный claim — claimed-before-grant

Гейт = **unique-insert** строки `mail_claims (mail_id, player_id)` в `claimed` **до** физической выдачи (01, Q6). `DuplicateKey` на insert ⇒ `AlreadyClaimed` — точное зеркало `_claims.InsertOneAsync` → `catch DuplicateKey` в `MongoDailyRewardBackend.ClaimAsync`. Крэш в окне после гейта до выдачи стоит одной доставки (лог), но никогда не дублирует (зеркало §6 «single-doc atomic, ordered so a crash can lose but never duplicate»).

### Маппинг вложений на игровую выдачу

Бэкенд выдаёт только `items`+`coins` (kit развёрнут в `items` на стороне vsservice at-compose — 01, Q1; до 05 kit не доходит). После insert-гейта, best-effort по-вложению (Q4): нерезолвимое вложение логируется+скипается (стоит того одного item), НЕ валит весь claim. `Unavailable` — только когда `ResolveUserIdAsync == null` (весь claim не стартует, нечего кредитовать). Мир мутируется на main thread (`EnqueueMainThreadTask`), бэкенд off-thread (`Task.Run`).

**items** — паттерн `MongoDailyRewardBackend.ResolveStack`:
1. `type` («item»/«block», Q1) → `_sapi.World.GetItem` / `GetBlock` по `AssetLocation(game_code)`. **Namespace НЕ угадывается** (два namespace держат один код) — тип берётся из вложения. Нерезолвимый код → лог + скип.
2. `attr_snapshot` (Q2): **пусто** ⇒ чистый стек (donate авто-item, 02); **непусто** ⇒ base64 бинарного `TreeAttribute` → `FromBytes` → `stack.Attributes.MergeTree` (совпадает с тем что capturekit захватывает из живого стека, формат Kit-системы). Битый снапшот ⇒ лог + выдать чистым (не валить claim).
3. Всегда после: `RemoveTransitionState(stack.Attributes)` + вложенный `contents.0` (еда не приходит преиспорченной — как `ResolveStack`).
4. `TryGiveItemstack(stack, true)` → fallback `SpawnItemEntity(stack, player.Entity.Pos.XYZ)`.

**coins** (Q6) — сырой Mongo `$inc` на `donate_wallets`, **воспроизводит upsert vsservice** `AddCoinsToWallet` (прочитан: `SetUpsert(true)`, `$setOnInsert: _id, created_at, version=1, player_name`). Поведение DailyRewards (дроп+лог при отсутствии кошелька) — **неверное зеркало** для broadcast-coins (тихо теряет валюту большинству). Бэкенд:
- условный `$inc coins` гард по `applied_claims ∌ claimId` (идемпотентность, как `CreditCoinsAsync`);
- `SetUpsert(true)` + `$setOnInsert` `_id` (`ObjectId`), `created_at`, **`version:1`** (критично — иначе backend-созданная строка сломает оптимистик-concurrency donate `UpdateWallet`, коммент wallet.go:47-50), `player_name = player.PlayerName`;
- аудит-строка в `donate_transactions` (`purchase_id = claimId`, `type = "credit"`, `reason = "mail:{mail_id}"`) — переиспользует существующий partial-unique индекс `purchase_id_type_unique_partial`, **не пересоздавать**.

### Коллекции + индексы (Q8)

Бэкенд читает vsservice Mongo напрямую (как `MongoDailyRewardBackend`):
- **`mail`**: `recipient, sender, title, body, attachments[], created_at, expires_at, revoked` (01).
- **`mail_claims`**: `mail_id, player_id, state, read_at, claimed_at`; unique `(mail_id, player_id)`.
- **`donate_wallets`** / **`donate_transactions`**: общие с donate/DailyRewards.

**vsservice владеет** индексами `mail`+`mail_claims` (писатель/владелец домена, 01). Бэкенд структурно ничем не владеет, но **защитно `TryCreateIndex`** (per-index try/catch, чтобы конфликт на общей коллекции не блокировал свои — паттерн `CreateIndexes`) на unique `(mail_id, player_id)` `mail_claims` (это и есть гейт claimed-before-grant). `donate_transactions` — **не** пересоздавать (donate/DailyRewards уже создали `purchase_id_type_unique_partial`).

### Marshalling / потоки

Роутер `MailModSystem` (06): `PlayerNowPlaying` не нужен как в DailyRewards (нет attendance) — push-снапшот на join + request-on-open. Бэкенд off-thread (`Task.Run`), сеть/мир на main thread (`EnqueueMainThreadTask`), Mongo-фолты логируются не бросаясь в хендлер (fail-closed, инбокс держит прошлый снапшот).

### Правки, откинутые на резолвленные тикеты

- **01**: примитив `ItemAttachment` **дополняется полем `type`** («item»/«block», дефолт «item») — `ResolveStack` требует объявленный namespace, угадывать нельзя (Q1). proto `ItemAttachment{game_code, quantity, attr_snapshot, type}`.
- **02**: donate авто-item эмитит `type="item"` (ShopItem всегда item), `AttrSnapshot=""`.
- **04**: `ItemAttachment` в `ComposeMail` несёт `type`; структурная валидация — `type in {item,block}`.

Разблокирует UI-тикет 06.
