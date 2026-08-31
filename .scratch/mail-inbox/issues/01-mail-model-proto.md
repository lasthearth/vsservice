---
Type: grilling
Status: resolved
Blocked by:
---

# Модель письма и proto `mail/v1` в vsservice

## Question

Определить доменный аггрегат `Mail` и proto-контракт `proto/mail/v1/` — форму, которую фиксируют все остальные тикеты.

Решить:
- **Поля письма**: id, recipient (`user_id` или `"broadcast"`), sender (система/админ id), title, body, state (unread/read/claimed), created_at, expires_at (nil = вечное), attachments.
- **Вложения** (список, письмо без вложений = уведомление): три типа —
  - items: game-code (item/block) + qty + base64 attr-снапшот (паттерн Kit/DailyRewards),
  - coins: сумма в donate wallet,
  - kit-ref: имя/ссылка кита целиком.
- **Стейт-машина**: допустимые переходы unread→read→claimed; claim только при наличии вложений; чистое уведомление сразу «прочитано/без claim».
- **Broadcast**: один документ письма с `user_id="broadcast"` + отдельная коллекция `mail_claims` (mail_id + player_id + claimed_at) для per-player трекинга прочтения/выдачи. Таргетное письмо — частный случай.
- **Срок годности**: `expires_at` + Mongo TTL-индекс; истёкшие не показываются.
- **Мутации только через методы модели** (конвенция CLAUDE.md); ошибки в `ierror`.
- **proto**: сервис `MailService`, сообщения Mail/Attachment/enum-ы; какие RPC (List/MarkRead/Claim) для игрока — админ-compose отдельным тикетом 04. Роуты `/v1/mail...`, документированные ошибки.

Резолюция фиксирует: список полей модели, набор proto-сообщений и enum-ов, стейт-машину, схему broadcast/mail_claims, стратегию TTL. Это разблокирует 02–05.

## Answer

Зафиксирована модель `Mail` и контракт `proto/mail/v1/`. Ключевые решения:

### Вложения — ровно ДВА примитива (было три; kit-ref удалён)

Письмо самодостаточно: любой бэкенд выдаёт вложение без внешнего лукапа.

- **`items`**: `game_code` (item/block) + `quantity` + `attr_snapshot` (base64 attribute-снимок, паттерн DailyRewards/Kit `ResolveStack`).
- **`coins`**: сумма в donate wallet (`$inc` в `donate_wallets`).

«Кит» вложением **не является**. Донат-кит разворачивается в список `items`-вложений **в момент compose** (когда game_code известны). Это требует, чтобы `KitEntry` нёс `game_code` + optional `attr_snapshot` — источник кодов даёт kit-builder (см. Q4/новые тикеты). До появления builder разворот кита блокируется данными, не моделью письма.

Отброшено: `kit-ref` как тип вложения (дырка в модели — невыразим без внешнего резолва, разного у двух источников); `grant-ref`/отложенная выдача (не нужен, раз кит разворачивается at-compose).

### Поля письма (иммутабельный контент-документ)

`id`, `recipient` (`user_id` | `"broadcast"`), `sender` (system/admin id), `title`, `body`, `attachments []Attachment`, `created_at`, `expires_at` (nil = вечное). **Стейт письма в документе НЕ хранится** — переехал в `mail_claims` (см. ниже). Документ письма иммутабелен после создания (кроме `revoked`-флага, см. стейт-машину).

### Per-player стейт — всегда через `mail_claims` (broadcast больше не спецслучай)

Коллекция `mail_claims`: `(mail_id, player_id)` unique, `state`, `read_at`, `claimed_at`. Каждый получатель (targeted ИЛИ broadcast) получает строку. Targeted = `mail_claims` с одной строкой. Один идемпотентный claim-путь — зеркало DailyRewards `ItemsGiven` / donate `applied_claims` (claimed-before-grant: строка в `claimed` пишется ДО физической выдачи).

### Стейт-машина (per-player, в `mail_claims`)

```
unread ─→ read ─→ claimed          (claimed только если у письма есть вложения)
   │        │
   ├────────┴─→ expired            (по expires_at, только непроклейменное; терминальное)
   └────────┴─→ revoked            (отзыв админом, только непроклейменное; терминальное)

уведомление (attachments пусто): create → read; claim недоступен.
```

- **`expired`** (Q5a): `expires_at` НЕ TTL-удаляет. Лениво at-read + фоновый sweep переводят непроклейменные истёкшие в `expired`; показываются серым «Срок истёк», claim заблокирован. TTL как дальняя garbage-collection на непоказываемые терминальные — отдельно, дальний горизонт (fog).
- **`revoked`** (Q5b): в модели сразу. Терминальное из непроклейменного. Broadcast: отзыв ставит письмо в `revoked` (флаг на документе) — непроклеймившие теряют доступ, уже проклеймившим НЕ откатывает (выдача необратима). Откат уже-выданного вне scope. Админ-RPC для revoke — ticket 04.

Claim только при непустых `attachments`. Мутации только через методы модели (конвенция CLAUDE.md), ошибки в `ierror`.

### proto `mail/v1`

- `MailService` (игрок): `ListMail`, `MarkRead`, `Claim` (+ `ClaimAll`). Маршруты `/v1/mail`, `/v1/mail/{id}:read`, `/v1/mail/{id}:claim`, документированные ошибки.
- Сообщения: `Mail`, `Attachment` (oneof `ItemAttachment` | `CoinsAttachment`), enum `MailState` (unread/read/claimed/expired/revoked).
- Админ-compose + revoke — отдельно, ticket 04.

Разблокирует 02, 04, 05. Ticket 03 (старый internal/kit) закрыт как out-of-scope.

### Поправка от тикета 05 (item-vs-block namespace)

`ItemAttachment` **дополняется полем `type`** («item»/«block», дефолт «item»). Причина: VintageAPI `ResolveStack` не угадывает namespace — один код может жить и как item, и как block (`_sapi.World.GetItem` vs `GetBlock`). Тип обязан ехать во вложении. proto: `ItemAttachment{game_code, quantity, attr_snapshot, type}`. `attr_snapshot` — base64 бинарного `TreeAttribute` (не JSON): пусто ⇒ чистый стек, непусто ⇒ `FromBytes`+`MergeTree` (формат Kit-системы / capturekit).
