---
Type: grilling
Status: resolved
Blocked by: 09
---

# Разворот кит-мейла at-compose: `kitExpander` + `ComposeKitMail`, снятие `type=kit` с ручного

## Question

Как донат-кит (`ShopItem type=kit`) и админ-broadcast «всем кит» разворачиваются из кит-определения (`kits`, контракт 09) в список `items`-вложений **в момент compose**, и как `type=kit` снимается с ручного `MarkIssued` (fog из тикета 02).

Решить (опираясь на контракт 09):
- **Кто разворачивает** (Q6=A): mail-домен, через внутренний `kitExpander`, читающий `kits` по `kit_id`. `donate` НЕ знает game_code — порт `MailComposer` (тикет 02) расширяется методом `ComposeKitMail(ctx, recipientPlayerID, kitID, title, body, purchaseID string) error`. mail читает `kits`, разворачивает `[]KitItemDoc`→`[]items-вложение` (`game_code`+`type`+`quantity`+`attr_snapshot` из 09), собирает письмо. `donate` шлёт только `kitID` (у `ShopItem` должна появиться связь shop-item→kit — какое поле: `ShopItem.KitID`? Сейчас `Entries []KitEntry` display-only без кода. Как ShopItem ссылается на канонический `kits`-документ).
- **Общий экспандер для админ-compose (04)**: тот же `kitExpander` нужен `ComposeMail`-админу для broadcast «всем кит». Внутренний сервис mail-домена, зовут оба пути (donate `ComposeKitMail` + админ). Форма: `expand(kitID) ([]Attachment, error)`.
- **Донат-развилка (снятие с ручного)**: тикет 02 оставил `type=kit` на ручном `MarkIssued` «блокировано данными». Теперь данные есть (`kits`). `Buy()` для `type=kit` зовёт `ComposeKitMail` вместо ручного пути; штампует `IssuedBy="system:mail"` как item-путь (02). Kit-purchases уходят с ручного `MarkIssued`.
- **Кит отсутствует/пуст at-compose**: `ShopItem.KitID` указывает на несуществующий/пустой `kits`-документ (кит ещё не захвачен) — compose падает (purchase есть, reconcile) или письмо без вложений? Как в 02 (idempotent на purchaseID).
- **attr в разворот**: `KitItemDoc.attr_snapshot` (base64, 07) едет как есть в `ItemAttachment.attr_snapshot` — mail не парсит, VintageAPI at-claim декодит (05). Подтвердить сквозной проброс.
- **Связь с админ-compose 04**: 04 сейчас принимает `repeated Attachment` (только items+coins, «kit не примитив запроса»). Как админ выбирает кит для broadcast — `ComposeMail` расширяется `oneof {attachments | kit_id}`, или отдельный `ComposeKitBroadcast(kit_id)`. Может потребовать правку тикета 04.

Резолюция фиксирует: расширение порта `MailComposer` (`ComposeKitMail`), внутренний `kitExpander`, связь `ShopItem→kits` (`KitID`), снятие `type=kit` с ручного `MarkIssued`, обработку отсутствующего кита, интеграцию с админ-compose 04. Разблокирует полное снятие ручной выдачи (fog 02).

## Answer

Развёрнут разворот кит-мейла at-compose. Ключ: **кит НИКОГДА не примитив вложения** — mail-домен читает `kits` (09) и разворачивает в `[]ItemAttachment` до записи письма; оба пути (donate-покупка + админ-broadcast) сходятся в один mail-internal `kitExpander`. Опирается на 09 (`GetKit(code)`, `KitItemDoc`), 02 (`MailComposer`-порт, seq-шаг, идемпотентность по `purchaseID`), 04 (админ-compose), 01 (`ItemAttachment{game_code,type,quantity,attr_snapshot}`, claim только при непустых attachments).

### `kitExpander` — внутренний сервис mail-домена (Q3)

- `kitExpander` живёт внутри mail, зовут оба пути (donate `ComposeKitMail` + админ `ComposeKitMail`-RPC). Форма: `expand(ctx, kitID string) ([]Attachment, error)`.
- Разворот `[]KitItemDoc`→`[]ItemAttachment`: проброс `game_code`+`type`+`quantity`+`attr_snapshot` **как есть**, байт-в-байт. `image_url` НЕ пробрасывается (вложение его не несёт, 01). mail не парсит base64.
- **Стрип `temperature`/`transitionstate` НЕ здесь** — снимок в `kits` честный, чистка at-claim в VintageAPI (05/09). Экспандер = чистый транспорт.

### Доступ mail→kitdef — consumer-порт `KitReader` (Q3=A)

mail определяет свой узкий порт (зеркало `MailComposer` из 02), ацикл, mail не импортирует kitdef-модель:

```go
type KitReader interface {
    GetKit(ctx context.Context, code string) (*KitSnapshot, error) // NotFound если не захвачен
}
type KitSnapshot struct { Items []KitItem }
type KitItem struct{ GameCode, Type, AttrSnapshot string; Quantity int32 }
```

`kitdef`-сервис (09) реализует `GetKit(code)`, связка `fx.As(new(mail...KitReader))`. Конвенция CLAUDE.md «интерфейс принадлежит потребителю».

### Расширение порта `MailComposer` — `ComposeKitMail` (Q4)

```go
type MailComposer interface {
    ComposeItemMail(ctx, recipientPlayerID, title, body, purchaseID string, items []ItemSpec) error // 02
    ComposeKitMail(ctx, recipientPlayerID, kitID, title, body, purchaseID string) error             // 11
}
```

`donate` шлёт только `kitID` (game_code не знает); mail внутри `expand(kitID)`→`[]Attachment`→собирает письмо. Идемпотентно по `purchaseID`.

### Связь `ShopItem→kits` (Q4.1)

`ShopItem.Code` (для `type=kit`) **==** `kits.code` (09). Нового поля у `ShopItem` НЕ вводим. `Buy()` для `type=kit` шлёт `item.Code` как аргумент `kitID`. `Code` двузначен по `Type`: item-код (`type=item`) / kit-слаг (`type=kit`). Сквозной проброс: `ShopItem.Code`→`ComposeKitMail(kitID)`→`expand(kitID)`→`GetKit(code)`.

### Снятие `type=kit` с ручного `MarkIssued` (Q4.4)

`Buy()` для `type=kit` зовёт `ComposeKitMail` **вместо** ручного пути (был `MarkIssued` «блокировано данными», 02 — теперь данные есть). Успешный compose→`Buy()` штампует `IssuedAt`+`IssuedBy="system:mail"` (как item в 02). Kit-purchases уходят с ручного пути. `MarkIssued`/`IssuedAt`/`IssuedBy` остаются deprecated (recovery/reconcile), не удаляем (полный демонтаж — fog 02).

### Кит отсутствует/пуст at-compose — fail-loud + reconcile (Q1=A)

`expand` возвращает ошибку когда `GetKit`→`NotFound` (кит не захвачен) **или** `items[]` пуст (`FailedPrecondition`). Пустой кит ≡ отсутствующий: НЕ создаём claimless kit-письмо (claim заблокирован при пустых attachments, 01 — иначе молчаливый провал). compose-шаг падает, `Buy()` возвращает err. Purchase УЖЕ создан (seq-шаг 2), `IssuedBy` НЕ штампуется → фоновый reconcile / ручной `MarkIssued` добирает, когда кит захватят. Идемпотентно по `purchaseID` (02): повтор безопасен. Механизм reconcile — fog 02 (не решаем здесь).

### attr сквозной проброс (Q4.3)

`KitItemDoc.attr_snapshot` (base64, 07)→`ItemAttachment.attr_snapshot` байт-в-байт; `game_code`+`type`+`quantity` тоже. mail не парсит, VintageAPI at-claim декодит (05). Стрип `temperature`/`transitionstate` — at-claim, не в экспандере.

### Интеграция с админ-compose (04) — отдельный RPC `ComposeKitMail` (Q2=A)

`ComposeMail` (04, `repeated Attachment`, «kit не примитив запроса») НЕ трогаем. Добавляем **отдельный** RPC в тот же `MailService`:

```proto
message ComposeKitMailRequest {
  oneof recipient { string user_id = 1; bool broadcast = 2; }
  string kit_id  = 3;
  string title   = 4;
  string body    = 5;
  optional google.protobuf.Timestamp expires_at = 6;
  optional string idempotency_key = 7;
}
message ComposeKitMailResponse { string mail_id = 1; }
```

- Route `POST /v1/mail:compose-kit`, scope `mail:compose` (тот же, что `ComposeMail`).
- Внутри — тот же `kitExpander`: `kit_id`→`[]Attachment` до записи письма. Ошибка отсутствующего/пустого кита → `NotFound`/`FailedPrecondition` (Q1).
- Идемпотентность по `idempotency_key` (04); donate-путь по `purchaseID` (02).
- oneof-с-`repeated` в proto3 неуклюж (нужна обёртка-message), потому отдельный RPC, а не расширение `ComposeMail`. Оба пути (donate-порт + админ-RPC) → один mail-internal `expand+compose`.
- **Требует аддитивной поправки тикета 04** (+метод +scope-строка +route), не переписывания.

### Разблокирует

Полное снятие ручной выдачи (kit-путь снят здесь, item — в 02; демонтаж `MarkIssued` после стабилизации — fog 02). Destination (спец) достигается когда 10 (игровой захват) + 11 пройдены.
