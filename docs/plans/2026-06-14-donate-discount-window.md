# План: временное окно скидки для donate shop item

## Цель
Дать скидке в donate-магазине срок действия (флеш-акция): поля `starts_at`/`ends_at`,
авто-включение/выключение по текущему времени. Без планировщика, без анонсов,
без отдельной сущности Promotion — всё внутри домена `donate`.

## Решения (зафиксированы)
1. **Семантика**: `has_discount` остаётся мастер-флагом. Скидка активна ⇔
   `HasDiscount && DiscountPercent > 0 && (start == nil || now >= start) && (end == nil || now < end)`.
   Пустое окно (оба nil) = текущее поведение (скидка всегда вкл).
2. **effective_price / discount_active**: считаются на чтении по `now()`. Не персистятся.
3. **Анонс/notification — вне scope.** Только цена + поля API + `ends_at` для таймера на фронте.

## Конвенции проекта (соблюдать)
- Модель мутируется ТОЛЬКО через свои методы (не присваивать поля снаружи).
- Интерфейсы репозитория живут в пакете-потребителе (service), не в repository.
- Маппинг через goverter; extend-функции в `internal/donate/internal/goverter/extend.go`.
- Не редактировать сгенерированный код в `/gen` вручную — регенерить.
- Коммиты БЕЗ `Co-Authored-By`.

---

## Изменения по файлам

### 1. `proto/donate/v1/shop_item.proto`
`google/protobuf/timestamp.proto` уже импортирован.

В `message ShopItem` добавить (следующие свободные номера — 16,17,18):
```proto
  google.protobuf.Timestamp discount_starts_at = 16; // nil = открыто слева
  google.protobuf.Timestamp discount_ends_at = 17;   // nil = открыто справа
  bool discount_active = 18;                          // вычисляемое по now(), для UI-бейджа
```

В `CreateShopItemRequest` добавить (свободные 11,12):
```proto
  google.protobuf.Timestamp discount_starts_at = 11;
  google.protobuf.Timestamp discount_ends_at = 12;
```

В `UpdateShopItemRequest` добавить (свободные 13,14):
```proto
  google.protobuf.Timestamp discount_starts_at = 13;
  google.protobuf.Timestamp discount_ends_at = 14;
```
Если у соседних полей есть protovalidate-аннотации (`buf.validate`), для новых полей
ничего обязательного не добавлять (окно опционально). Затем `buf generate`.

### 2. `internal/donate/internal/model/shop_item.go`
- В `ShopItem` добавить поля:
  ```go
  DiscountStartsAt *time.Time
  DiscountEndsAt   *time.Time
  ```
- В `ShopItemUpdate` добавить те же два поля.
- В `Apply` присвоить их из `ShopItemUpdate`.
- Рефактор `EffectivePrice`:
  ```go
  // EffectivePriceAt returns the price after applying the discount, if active at `now`.
  func (s *ShopItem) EffectivePriceAt(now time.Time) int64 {
      if !s.DiscountActive(now) {
          return s.Price
      }
      p := s.Price * int64(100-s.DiscountPercent) / 100
      if p < 1 {
          p = 1
      }
      return p
  }

  // EffectivePrice keeps backward-compat (uses time.Now()).
  func (s *ShopItem) EffectivePrice() int64 { return s.EffectivePriceAt(time.Now()) }

  // DiscountActive reports whether the discount applies at `now`.
  func (s *ShopItem) DiscountActive(now time.Time) bool {
      if !s.HasDiscount || s.DiscountPercent <= 0 {
          return false
      }
      if s.DiscountStartsAt != nil && now.Before(*s.DiscountStartsAt) {
          return false
      }
      if s.DiscountEndsAt != nil && !now.Before(*s.DiscountEndsAt) { // now >= end -> неактивна
          return false
      }
      return true
  }
  ```
- В `Validate` добавить проверку окна:
  ```go
  if s.DiscountStartsAt != nil && s.DiscountEndsAt != nil &&
      !s.DiscountEndsAt.After(*s.DiscountStartsAt) {
      return errors.New("discount_ends_at must be after discount_starts_at")
  }
  ```
- Опционально метод `SetDiscountWindow(start, end *time.Time)` для мутации окна,
  если в service удобнее, чем через `Apply`/`ShopItemUpdate`.

### 3. `internal/donate/internal/dto/mongo/shop_item.go`
Добавить в `ShopItem` (нужен импорт `time`):
```go
DiscountStartsAt *time.Time `bson:"discount_starts_at,omitempty"`
DiscountEndsAt   *time.Time `bson:"discount_ends_at,omitempty"`
```

### 4. goverter mappers
- `sermapper/mapper.go` и `service/mapper.go` — модель↔dto (`*time.Time`↔`*time.Time`)
  мапятся напрямую, extend не нужен.
- модель↔proto: нужны конвертеры `*time.Time`↔`*timestamppb.Timestamp`.
  Добавить в `internal/donate/internal/goverter/extend.go`:
  ```go
  func TimePtrToTimestamp(t *time.Time) *timestamppb.Timestamp {
      if t == nil {
          return nil
      }
      return timestamppb.New(*t)
  }
  func TimestampToTimePtr(ts *timestamppb.Timestamp) *time.Time {
      if ts == nil {
          return nil
      }
      t := ts.AsTime()
      return &t
  }
  ```
  Импорты: `time`, `google.golang.org/protobuf/types/known/timestamppb`.
- Прописать `goverter:extend` для этих функций над интерфейсом маппера (как сделано для
  `ShopItemEffectivePrice`), чтобы goverter применил их к `discount_starts_at`/`discount_ends_at`.
- `discount_active`: НЕ мапить из модели (оно now-aware) — оставить дефолт, перетрётся в service.
- `effective_price`: оставить как есть (goverter тянет `EffectivePrice()`), но в service
  перетереть значением с общим `now` (см. ниже), чтобы был единый источник времени.
- Запустить `go generate ./...` в пакете с `//go:generate` (donate service mapper).

### 5. `internal/donate/internal/service/service.go`
- **CreateShopItem** (~стр. 90): после создания модели проставить окно из запроса
  (`TimestampToTimePtr(req.DiscountStartsAt)` и т.д.) через метод модели/`ShopItemUpdate`,
  до `item.Validate()`.
- **UpdateShopItem** (~стр. 160-178): добавить окно в формируемый `ShopItemUpdate`
  (через конвертацию из `req`), оно попадёт в `item.Apply(u)`; учесть взаимодействие с
  `ClearDiscount()` (если скидку убирают — окно тоже сбросить).
- **ListShopItems** (~стр. 308) и ответы Create/Update: после маппинга модель→proto
  для каждого item проставить с единым `now := time.Now()`:
  ```go
  pb.DiscountActive = m.DiscountActive(now)
  pb.EffectivePrice = m.EffectivePriceAt(now)
  ```
  Вынести в маленький хелпер, чтобы не дублировать в трёх местах.

### 6. `internal/donate/internal/repository/mongo/atomic.go` (КРИТИЧНО)
В `BuyItem` (~стр. 44) заменить расчёт на time-aware, иначе спишут цену по истёкшей/будущей скидке:
```go
now := time.Now()
eff := item.EffectivePriceAt(now)
discountPercent := int32(0)
if item.DiscountActive(now) {
    discountPercent = item.DiscountPercent
}
```
Добавить импорт `time`, если нет.

### 7. Тесты — `internal/donate/internal/model/shop_item_test.go`
Добавить кейсы для `DiscountActive`/`EffectivePriceAt` (передавать явный `now`):
- окно nil/nil → активна (как сейчас);
- `now` до `start` → неактивна, цена = `Price`;
- `now` в окне → активна, цена со скидкой;
- `now` == `end` и после → неактивна (граница: end эксклюзивна);
- `HasDiscount=false` при заданном окне → неактивна.
И тест `Validate`: `end <= start` → ошибка.

---

## Верификация (запустить и убедиться, что зелёное)
```bash
buf generate
go generate ./...
go build ./...
go test ./internal/donate/...
```
Проверить, что в `gen/donate/v1/shop_item.pb.go` появились новые поля и что
`docs/v1/openapi.yaml` перегенерился с ними.

## Вне scope (не делать)
- notification/news анонс старта акции;
- отдельная сущность/коллекция `Promotion`, кампании «1 акция → много товаров»;
- планировщик/cron сброса скидок;
- новый отдельный gRPC-эндпоинт под промо (поля идут в Create/Update).
