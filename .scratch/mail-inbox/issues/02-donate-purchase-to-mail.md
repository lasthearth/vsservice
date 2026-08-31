---
Type: grilling
Status: resolved
Blocked by: 01
---

# Донат-покупка → письмо (авто), вывод ручного MarkIssued

## Question

Как `donate.Buy()` кладёт письмо-с-вложением вместо ручной выдачи, и как выводится ручной путь `MarkPurchaseIssued`/`MarkIssued`.

Решить:
- В каком месте флоу `Buy()` (internal/donate/internal/usecase/buy.go) создаётся письмо: новый шаг в `seq.Do(...)` после `CreatePurchase`, или отдельный consumer. Атомарность/компенсация при частичном сбое (текущий Buy уже описывает семантику mid-sequence failure — сохранить её).
- Как donate-домен вызывает mail-домен: прямой fx-инжект mail-сервиса в donate, или общий репозиторий/интерфейс. Не ввести цикл зависимостей.
- Маппинг ShopItem → attachment: обычный предмет (`type=item`) → одно `items`-вложение (`code`+qty). Кит (`type=kit`) → **список** `items`-вложений, развёрнутый из `KitEntry` (тикет 01, Q1=B — kit-ref отброшен). **Блокер данных:** `KitEntry` сейчас несёт только display (`Name/Description/ImageURL/Quantity`), не `game_code`. Коды даёт kit-builder (fog в карте) — до него разворот кита неполон. Обычный item-путь не блокирован.
- Что письмо содержит в title/body (название предмета, цена, скидка-снимок).
- Deprecation-план: `MarkIssued`/`MarkPurchaseIssued` и `IssuedAt`/`IssuedBy` — помечаем выводимыми (не удаляем в этой карте), что с исторrunning purchases.

Резолюция: точка интеграции, направление зависимости, маппинг item/kit→attachment, план вывода ручной выдачи.

## Answer

### Точка интеграции (Q1)

Письмо создаётся **четвёртым шагом в существующем `seq.Do(...)`** в `Buy()`, после `CreatePurchase`. Никакого отдельного consumer — NATS вне scope (карта). `seq` даёт session-scoped read-your-writes; семантика mid-sequence failure сохраняется: purchase — запись-истины, письмо — доставка.

### Направление зависимости (Q2)

`donate → mail`, через узкий consumer-порт в `donate/usecase`, примитивно-типизированный (зеркало `donateuc.WalletRepo` / `AddCoinsUseCase`):

```go
type MailComposer interface {
    // идемпотентно по purchaseID (Q4)
    ComposeItemMail(ctx, recipientPlayerID, title, body, purchaseID string, items []ItemSpec) error
}
type ItemSpec struct{ GameCode string; Quantity int32; AttrSnapshot string }
```

Реализуется mail-доменом, связывается через fx `fx.As(new(usecase.MailComposer))`. **mail НЕ импортирует donate** (coins-вложение резолвит VintageAPI at-claim, не donate at-compose) → цикла нет. `ItemSpec` (attachment-спека, которую строит donate) — то, что питает 04/05.

### Маппинг ShopItem → attachment + кит-развилка (Q3)

Развилка по `ItemType` в `Buy()`:
- **`type=item`** → авто-письмо СЕЙЧАС. Одно `items`-вложение `{GameCode: item.Code, Quantity: 1, AttrSnapshot: ""}`. У ShopItem есть `Code` (game-asset correlation), attr-снимка шоп не несёт → пусто.
- **`type=kit`** → письмо НЕ создаётся; кит остаётся на ручном пути `MarkIssued` до kit-builder. **Блокер данных:** `KitEntry` несёт только display (`Name/Description/ImageURL/Quantity`), без `game_code`. Разворот кита в `items`-вложения дозреет отдельным тикетом после kit-builder storage (fog).

### Контент письма (Q4)

- `recipient` = `playerID` (targeted, одна строка `mail_claims`).
- `sender` = `"system:donate"`.
- `title` = `"Покупка: {ItemName}"`.
- `body` = снимок цены: `PricePaid` / `BasePrice` / `DiscountPercent` (история не переписывается позднее — как на purchase).
- `expires_at` = `nil` (оплаченное клеймится вечно).
- Идемпотентность: `ComposeItemMail` идемпотентен по ключу `purchaseID` (unique). При сбое compose-шаг возвращает ошибку; purchase уже существует, повторный/фоновый reconcile безопасен (idempotent на purchaseID). **Механизм reconcile — fog** (в этой карте не решаем).

### Вывод ручной выдачи (Q5)

`MarkIssued` / `MarkPurchaseIssued` / `IssuedAt` / `IssuedBy` — **deprecate, не удалять** (карта). В переходный период:
- Авто-composed **item**-покупки штампуют `IssuedAt` + `IssuedBy = "system:mail"` в том же seq-шаге → инвариант «доставлено?» в админ-списке остаётся верным без новых полей и без правки админ-UI.
- **Kit**-покупки продолжают ручной `MarkIssued` (Q3).
- Recovery/ре-выдача — тоже ручной путь пока.
- Бэкфилл историчных running purchases (куплено, не выдано) — **fog**, отдельно.

### Поправка от тикета 05

`ItemSpec`/`ItemAttachment` получает поле `type` (01, поправка). Donate авто-item эмитит `type="item"` (ShopItem всегда item), `AttrSnapshot=""` (чистый стек). Одно `items`-вложение `{GameCode: item.Code, Quantity: 1, AttrSnapshot: "", Type: "item"}`.
