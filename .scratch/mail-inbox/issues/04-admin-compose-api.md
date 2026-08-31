---
Type: grilling
Status: resolved
Blocked by: 01
---

# Админ-compose API (таргет + broadcast)

## Question

RPC для ручной админ-рассылки писем: конкретному игроку или всем (broadcast). Покрывает и системные/событийные награды (частный случай того же API). **Единственная точка создания писем извне donate-домена** (тикет 01, Q7=C: запись писем только через vsservice). Игровой команды выдачи нет (Q8) — все ручные награды/киты (напр. pre-wipe «всем кит на еду» = broadcast) идут отсюда, с сайта.

Решить:
- Сигнатура RPC(ов): один `ComposeMail` с полем recipient (`user_id` | broadcast-флаг), или раздельные. Поля: title, body, attachments (items/coins/kit-ref), expires_at.
- Права: scoper-паттерн — какой JWT scope/claim требуется для compose (админ). Регистрация через `group:"scopers"`.
- Валидация: непустой title/body, валидность game-code во вложениях (или отложенная валидация на стороне VintageAPI при выдаче), лимиты на count/размер broadcast.
- Как broadcast материализуется: один документ `user_id="broadcast"` (см. тикет 01) — API просто ставит recipient=broadcast, фан-аут не нужен.
- Маршрут `/v1/mail:compose` (админ), документированные ошибки.

**Также в этом тикете (тикет 01, Q5b):** `RevokeMail` RPC — отзыв письма/награды до клейма. Broadcast: ставит `revoked`-флаг на документ, непроклеймившие теряют доступ, уже проклеймившим не откатывает. Требует того же админ-scope. Маршрут `/v1/mail/{id}:revoke`.

Резолюция: сигнатуры compose-RPC, требуемые scopes, правила валидации, форма broadcast-запроса.

## Answer

Админ-методы `ComposeMail` + `RevokeMail` — часть **того же** `MailService` (01), отделены от игроцких `ListMail/MarkRead/Claim/ClaimAll` только scope (Q9, зеркало donate: self-service + admin в одном сервисе). Единственная точка создания писем извне donate-домена (Q7=C тикета 01). Игра письма НЕ создаёт (⚠️-callout, Q8=нет игровой команды).

### ComposeMail — сигнатура (Q1, Q2, Q3)

Один RPC, `recipient` через oneof; targeted и broadcast единообразны (Q1):

```proto
message ComposeMailRequest {
  oneof recipient {
    string user_id   = 1;  // targeted
    bool   broadcast = 2;  // ставит recipient="broadcast" на документе (01)
  }
  string title      = 3;
  string body       = 4;
  repeated Attachment attachments = 5;  // тот же Attachment из 01: oneof ItemAttachment|CoinsAttachment
  optional google.protobuf.Timestamp expires_at = 6;  // nil = вечное (Q6)
  optional string idempotency_key = 7;                // retry-safe (Q7)
}
message ComposeMailResponse { string mail_id = 1; }
```

- **Вложения = тот же примитив на всех швах** (Q2): переиспользуем proto-`Attachment` из 01 (`ItemAttachment{game_code,quantity,attr_snapshot}` = `ItemSpec` из 02, `CoinsAttachment{amount}`). Никакого дубля. Пустой список = чистое уведомление (валидно, 01).
- **Kit сейчас НЕ примитив запроса** (Q3): API принимает только `items`+`coins`. kit-ref отброшен в модели (01) — вводить его в запрос = вернуть ту же дырку. Админ pre-wipe «всем кит» пока перечисляет `items` вручную. Удобный kit-разворот в `items` (когда `KitEntry` понесёт `game_code`) — отдельный тикет после kit-builder storage (fog); **шов API не меняется**, кит разворачивается в те же `items` до записи письма.
- **Фан-аута нет** (01): broadcast=true → один документ `recipient="broadcast"`, per-player `mail_claims` материализуются лениво при чтении/claim (05).

### RevokeMail — сигнатура и семантика (Q8)

```proto
message RevokeMailRequest  { string mail_id = 1; }
message RevokeMailResponse {}
```

- Ставит флаг `revoked` **на документ письма** (не на claims), targeted и broadcast единообразно (01).
- Эффект (01): непроклеймившие теряют доступ (лениво/at-read → статус `revoked`); уже проклеймившим НЕ откатывает (выдача необратима, откат вне scope).
- Терминальный только из непроклейменного (01).
- **Идемпотентен**: повторный revoke уже-revoked письма = no-op success.
- Ошибки: `NotFound` (нет письма). Уже-целиком-проклеймленное — success no-op (флаг ставится, но никого не задевает), не ошибка.

### Sender-identity (Q4)

`sender = "admin:{jwt_sub}"` — субъект JWT из claims. Аудит «кто разослал»; формат `admin:*` зеркалит `system:*` (donate auto = `system:donate`, 02). Событийные авто (fog) позже — свой `system:event:*`.

### Валидация (Q5, Q6)

vsservice валидирует только **структуру**, не семантику game-ассетов (каталогом не владеет):
- непустой `title`/`body`;
- `ItemAttachment.quantity > 0`, непустой `game_code`-строка;
- `CoinsAttachment.amount > 0`;
- `expires_at` если задан — строго в будущем;
- oneof `recipient` обязателен (ровно один из user_id/broadcast).

**Существование/валидность `game_code` НЕ проверяется at-compose** (Q5): каталог только у игры. Невалидный код всплывает at-claim в VintageAPI (05) → статус `Unavailable`. Совпадает с «письмо самодостаточно» (01): бэкенд выдаёт без внешнего лукапа, но проверка ассета там же, где выдача.

### Идемпотентность (Q7)

Опциональный клиент-`idempotency_key`. Повтор с тем же ключом → то же письмо, дубль не создаётся (broadcast «всем кит» по двойному клику необратим). Пустой ключ → каждый вызов новое письмо (осознанный выбор). Ответ несёт `mail_id`. Механизм хранения ключа (unique-индекс на письме, зеркало `purchase_id`-идемпотентности из 02) — деталь реализации 01-модели.

### Scopes (Q10)

Scoper-паттерн (`group:"scopers"`, реализовать `interceptor.Scoper` на mail-сервисе). srvName `/mail.v1.MailService/`:

| Method | Scope |
|---|---|
| `ComposeMail` | `mail:compose` |
| `RevokeMail` | `mail:revoke` |
| `ListMail` / `MarkRead` / `Claim` / `ClaimAll` | `interceptor.ScopeAuthenticated` |

Раздельные `mail:compose`/`mail:revoke` (гранулярность как donate `donate:shop:create`/`donate:purchase:refund`); отзыв деструктивнее — отделён от рассылки. Игроцкие = self-service (JWT-субъект = получатель).

### Маршруты (документированные)

- `POST /v1/mail:compose` (админ, `mail:compose`)
- `POST /v1/mail/{mail_id}:revoke` (админ, `mail:revoke`)

Игроцкие роуты (`/v1/mail`, `/v1/mail/{id}:read`, `/v1/mail/{id}:claim`) — из 01.

### Форма для 05

`ComposeMail` пишет `items`+`coins`-вложения (тот же `Attachment` из 01) — ровно то, что читает контракт доставки 05. Kit никогда не доходит до 05 как отдельный тип (развёрнут в `items` at-compose).

### Поправка от тикета 05

`ItemAttachment` получает поле `type` (01, поправка) — `ResolveStack` в VintageAPI не угадывает namespace. Структурная валидация дополняется: `type in {item, block}` (дефолт «item»). Шов `ComposeMail` не меняется, только форма примитива.

### Поправка от тикета 11 (админ-broadcast кита)

Kit-broadcast «всем кит» больше НЕ требует ручного перечисления `items` (было в Q3). Тикет 11 добавляет **отдельный** RPC `ComposeKitMail` в тот же `MailService` — `ComposeMail` (этот тикет) НЕ меняется:

```proto
message ComposeKitMailRequest {
  oneof recipient { string user_id = 1; bool broadcast = 2; }
  string kit_id = 3; string title = 4; string body = 5;
  optional google.protobuf.Timestamp expires_at = 6;
  optional string idempotency_key = 7;
}
message ComposeKitMailResponse { string mail_id = 1; }
```

Route `POST /v1/mail:compose-kit`, scope `mail:compose` (тот же). Внутри — mail-internal `kitExpander` (11): `kit_id`→`[]Attachment` (из `kits`, 09) до записи письма; отсутствующий/пустой кит → `NotFound`/`FailedPrecondition`. oneof-с-`repeated` в `ComposeMail` отвергнут (неуклюж в proto3), потому sibling-RPC. Scopes-таблица дополняется: `ComposeKitMail` → `mail:compose`.
