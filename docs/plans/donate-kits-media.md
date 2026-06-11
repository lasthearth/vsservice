# План: продажа наборов (китов) в donate + общий media-домен для presigned-загрузки

Статус: план готов к реализации. Реализует **Sonnet** строго по этому документу.
Решения зафиксированы через /council — НЕ переобсуждать, реализовать как описано.

## Ограничения для исполнителя

- НЕ делать `git commit`/`push`, не создавать ветки/PR. Только правки рабочей копии.
- НЕ добавлять `Co-Authored-By` нигде.
- Конвенции проекта (CLAUDE.md): интерфейсы репозиториев — в пакете сервиса; **модели меняются только через свои методы**; fx-wiring в `fx.go` + импорт в `main.go`. Generated-код (`gen/`, `*.pb.go`) не править руками — регенерировать через `buf generate`.
- Donate-`Mapper` написан **вручную** (`internal/donate/internal/service/mapper.go`, `MapperImpl`) — это НЕ goverter. Правим руками. `go generate` для donate не нужен.

## Итоговый функционал

1. `ShopItem` становится полиморфным: `item` или `kit`. У `kit` есть витринный список позиций (`entries`: название, описание, картинка, количество).
2. Скидка процентом на `ShopItem`: `has_discount` + `discount_percent` (0..100). Базовая `Price` неизменна, итог считается методом `EffectivePrice()`.
3. Покупка кита идёт тем же флоу, что предмет (`BuyItem` → `Purchase`), выдача **ручная** через `MarkPurchaseIssued`. Ничего нового для выдачи.
4. В `Purchase` фиксируется снимок `BasePrice` + `DiscountPercent` (помимо уже существующего `PricePaid`).
5. Картинки грузятся на S3 через **presigned PUT URL**, выдаваемые новым общим доменом `media`. donate принимает уже готовые `image_url` строками и валидирует их хост.
6. Домен `kit` выводится из обращения: `kit.App` убирается из `main.go`, код домена остаётся в репозитории.

---

## Порядок сборки (по шагам)

### Шаг 1. `pkg/storage`: presigned PUT

Файл `internal/pkg/storage/storage.go` — добавить метод:

```go
func (s *Storage) PresignedPutURL(
	ctx context.Context,
	bucketName, objectName string,
	expiry time.Duration,
) (string, error) {
	u, err := s.client.PresignedPutObject(ctx, bucketName, objectName, expiry)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}
```

Добавить импорт `time`. (Content-type строго presigned PUT не пинит — это ограничение minio `PresignedPutObject`; клиент обязан слать `image/webp`. Строгая привязка потребовала бы presigned POST policy — вне скоупа.)

### Шаг 2. Новый домен `media`

Структура (по layout проекта):

```
proto/media/v1/media.proto
internal/media/fx.go
internal/media/internal/service/app.go      # Service, Opts, New, Storage iface
internal/media/internal/service/service.go   # CreateUploadUrls
internal/media/internal/service/scope.go     # Scope()
```

**proto/media/v1/media.proto:**

```protobuf
syntax = "proto3";

package media.v1;

import "google/api/annotations.proto";

// Media service — выдаёт presigned PUT URL для прямой загрузки в S3.
service MediaService {
  // Admin: получить N presigned-ссылок для загрузки картинок.
  //
  // Errors:
  //   - INVALID_ARGUMENT (400): неизвестный purpose или count вне [1,20]
  //   - UNAUTHENTICATED (401)
  //   - PERMISSION_DENIED (403)
  //   - INTERNAL (500)
  rpc CreateUploadUrls(CreateUploadUrlsRequest) returns (CreateUploadUrlsResponse) {
    option (google.api.http) = {
      post: "/v1/media/upload-urls"
      body: "*"
    };
  }
}

// Назначение загрузки — маппится на конкретный бакет на стороне сервера.
// Клиент НЕ передаёт имя бакета напрямую (защита от записи в произвольный бакет).
enum UploadPurpose {
  UPLOAD_PURPOSE_UNSPECIFIED = 0;
  UPLOAD_PURPOSE_DONATE_SHOP = 1;
}

message CreateUploadUrlsRequest {
  UploadPurpose purpose = 1;
  int32 count = 2;
}

message UploadTarget {
  // PUT-ссылка: клиент льёт сюда тело картинки методом PUT.
  string put_url = 1;
  // Публичный URL объекта после загрузки — его клиент передаёт в donate.
  string public_url = 2;
}

message CreateUploadUrlsResponse {
  repeated UploadTarget targets = 1;
}
```

**app.go** — по образцу `internal/donate/internal/service/app.go`:

```go
package service

import (
	mediav1 "github.com/lasthearth/vsservice/gen/media/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/storage"
	"go.uber.org/fx"
	"time"
)

var _ mediav1.MediaServiceServer = (*Service)(nil)

const presignExpiry = 15 * time.Minute

// purpose enum → bucket. Только из этого whitelist; неизвестный purpose = ошибка.
var purposeBuckets = map[mediav1.UploadPurpose]string{
	mediav1.UploadPurpose_UPLOAD_PURPOSE_DONATE_SHOP: "donate-shop",
}

type Storage interface {
	PresignedPutURL(ctx context.Context, bucketName, objectName string, expiry time.Duration) (string, error)
}

var _ Storage = (*storage.Storage)(nil)

type Service struct {
	storage Storage
	cfg     config.Config
	log     logger.Logger
}

type Opts struct {
	fx.In
	Storage Storage
	Config  config.Config
	Logger  logger.Logger
}

func New(opts Opts) *Service {
	return &Service{storage: opts.Storage, cfg: opts.Config, log: opts.Logger}
}
```
(импорт `context` добавить.)

**service.go** — `CreateUploadUrls`:
- валидация: `count` в [1,20]; `bucket, ok := purposeBuckets[req.Purpose]`, иначе `InvalidArgument`.
- цикл count раз: `id := uuid.NewV7()`, `objectName := id.String()+".webp"`, `putURL, err := s.storage.PresignedPutURL(ctx, bucket, objectName, presignExpiry)`, `publicURL := fmt.Sprintf("%s/%s/%s", s.cfg.CdnUrl, bucket, objectName)`.
- вернуть `[]*mediav1.UploadTarget`.

**scope.go** — по образцу donate:
```go
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/media.v1.MediaService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "CreateUploadUrls"): interceptor.Scope("media:upload:create"),
	}
}
```

**fx.go** — по образцу `internal/donate/fx.go` (без OnStart-хука бакета — бакет создаёт donate):
```go
var App = fx.Options(fx.Module("media",
	fx.Decorate(func(l logger.Logger) logger.Logger { return l.WithScope("media") }),
	fx.Provide(fx.Private,
		fx.Annotate(pkgstorage.New, fx.As(new(service.Storage))),
	),
	fx.Provide(
		fx.Annotate(service.New, fx.As(new(mediav1.MediaServiceServer))),
		fx.Annotate(service.New, fx.As(new(interceptor.Scoper)), fx.ResultTags(`group:"scopers"`)),
	),
))
```

`buf generate` после добавления proto.

### Шаг 3. Регистрация media в server

`internal/server/app.go`:
- импорт `mediav1 "github.com/lasthearth/vsservice/gen/media/v1"`.
- в `Opts`: `MediaV1 mediav1.MediaServiceServer`.
- в `Server`: поле `mediaV1 mediav1.MediaServiceServer`.
- в `New`: `mediaV1: opts.MediaV1`.

`internal/server/server.go`:
- импорт `mediav1`.
- в `Run`: `mediav1.RegisterMediaServiceServer(srv, s.mediaV1)`.
- в `RunInProcessGateway`: `mediav1.RegisterMediaServiceHandlerFromEndpoint(ctx, mux, grpcaddr, dopts)` с обёрткой ошибки.

`main.go`: добавить `media.App` в список fx-модулей (импорт `internal/media`).

### Шаг 4. proto donate

`proto/donate/v1/shop_item.proto`:

```protobuf
enum ItemType {
  ITEM_TYPE_UNSPECIFIED = 0;
  ITEM_TYPE_ITEM = 1;
  ITEM_TYPE_KIT = 2;
}

message KitEntry {
  string name = 1;
  string description = 2;
  string image_url = 3;
  int32 quantity = 4;
}
```

В `ShopItem` добавить:
```protobuf
  ItemType item_type = 10;
  repeated KitEntry entries = 11;
  bool has_discount = 12;
  int32 discount_percent = 13;   // 0..100
  int64 effective_price = 14;    // вычисляемое, для UI
```

`CreateShopItemRequest`: заменить `bytes image = 3;` на `string image_url = 3;`. Добавить:
```protobuf
  ItemType item_type = 6;
  repeated KitEntry entries = 7;
  bool has_discount = 8;
  int32 discount_percent = 9;
```

`UpdateShopItemRequest`: заменить `bytes image = 4;` на `string image_url = 4; // пусто = оставить текущую`. Добавить:
```protobuf
  ItemType item_type = 8;
  repeated KitEntry entries = 9;
  bool has_discount = 10;
  int32 discount_percent = 11;
```

`proto/donate/v1/purchase.proto` — в `Purchase` добавить:
```protobuf
  int64 base_price = 12;
  int32 discount_percent = 13;
```

`buf generate`.

### Шаг 5. model donate

`internal/donate/internal/model/shop_item.go`:

```go
type ItemType string
const (
	ItemTypeItem ItemType = "item"
	ItemTypeKit  ItemType = "kit"
)

type KitEntry struct {
	Name        string
	Description string
	ImageURL    string
	Quantity    int32
}
```

В `ShopItem` добавить поля: `Type ItemType`, `Entries []KitEntry`, `HasDiscount bool`, `DiscountPercent int32`.

`NewShopItem` — выставлять `Type: ItemTypeItem` по умолчанию. Добавить `NewKitShopItem(code, name, description, imageURL string, price int64, entries []KitEntry)`.

Методы (мутаторы — менять только через них):
```go
func (s *ShopItem) SetDiscount(percent int32) error  // 0..100; ставит HasDiscount=true
func (s *ShopItem) ClearDiscount()                    // HasDiscount=false, DiscountPercent=0
func (s *ShopItem) SetEntries(e []KitEntry) error     // валидирует quantity>0, name!=""
func (s *ShopItem) EffectivePrice() int64             // см. ниже
```

`EffectivePrice`:
```go
if !s.HasDiscount || s.DiscountPercent <= 0 {
	return s.Price
}
p := s.Price * int64(100-s.DiscountPercent) / 100
if p < 1 { p = 1 }   // clamp
return p
```

`Validate()` дополнить:
- `DiscountPercent` ∈ [0,100]; если `HasDiscount==false` — percent игнорируется (можно занулить в `ClearDiscount`).
- если `Type==ItemTypeKit`: `Entries` непустой, у каждого `Name!=""` и `Quantity>0`.

`Update(...)` НЕ раздувать новыми аргументами. Рекомендация: ввести options-struct и метод
```go
type ShopItemUpdate struct {
	Code, Name, Description, ImageURL string
	Price int64
	IsAvailable bool
	Type ItemType
	Entries []KitEntry
	HasDiscount bool
	DiscountPercent int32
}
func (s *ShopItem) Apply(u ShopItemUpdate) { ... }
```
и обновить вызов в `service.UpdateShopItem`. (Старый `Update` можно удалить — он используется только там; проверить grep.)

`internal/donate/internal/model/purchase.go`:
- В `Purchase` добавить `BasePrice int64`, `DiscountPercent int32`.
- Изменить `NewPurchase`: `NewPurchase(playerID, playerName, itemID, itemName string, pricePaid, basePrice int64, discountPercent int32)`. Заполнять `PricePaid: pricePaid, BasePrice: basePrice, DiscountPercent: discountPercent`.

### Шаг 6. dto/mongo donate

`internal/donate/internal/dto/mongo/shop_item.go`:
```go
type KitEntryDTO struct {
	Name        string `bson:"name"`
	Description string `bson:"description"`
	ImageURL    string `bson:"image_url"`
	Quantity    int32  `bson:"quantity"`
}
```
В `ShopItem` добавить:
```go
	Type            string        `bson:"type"`               // без omitempty: lazy-migration
	Entries         []KitEntryDTO `bson:"entries,omitempty"`
	HasDiscount     bool          `bson:"has_discount"`
	DiscountPercent int32         `bson:"discount_percent"`
```

`internal/donate/internal/dto/mongo/purchase.go` — добавить:
```go
	BasePrice       int64 `bson:"base_price"`
	DiscountPercent int32 `bson:"discount_percent"`
```

### Шаг 7. repository/mongo donate

`internal/donate/internal/repository/mongo/app.go`:
- `shopItemFromDTO`: маппить новые поля; **дефолт типа**:
  ```go
  t := model.ItemType(d.Type)
  if t == "" { t = model.ItemTypeItem }   // lazy-migration старых доков
  ```
  плюс конвертация `Entries`.
- `shopItemToDTO`: маппить `Type`, `Entries`, `HasDiscount`, `DiscountPercent`. Если `m.Type==""` ставить `item`.
- `purchaseFromDTO` / `purchaseToDTO`: добавить `BasePrice`, `DiscountPercent`.

`internal/donate/internal/repository/mongo/shopitem.go` — `CreateShopItem`:
- **ИСПРАВИТЬ существующий баг**: сейчас inline-DTO не пишет `Code`. Перевести построение DTO на `shopItemToDTO(item)` + проставить туда `mongox.NewModel()` envelope, либо добавить недостающие поля (`Code`, `Type`, `Entries`, `HasDiscount`, `DiscountPercent`) в inline-DTO. Проще: использовать `shopItemToDTO` и затем `d.Model = mongox.NewModel()`.

### Шаг 8. atomic.go (BuyItem) — снимок цены

`internal/donate/internal/repository/mongo/atomic.go`, `BuyItem`:
- заменить `w.Withdraw(item.Price)` на `w.Withdraw(item.EffectivePrice())`.
- заменить создание purchase:
  ```go
  eff := item.EffectivePrice()
  p, err := r.createPurchase(sc, model.NewPurchase(
      playerID, playerName, item.Id, item.Name,
      eff, item.Price, discountPercentOf(item)))
  ```
  где `discountPercentOf` = `item.DiscountPercent` если `HasDiscount`, иначе 0.
- транзакция: `model.NewDebitTransaction(playerID, eff, "purchase: "+item.Name)`.

`Refund` НЕ трогать — он уже корректно возвращает `p.PricePaid`.

### Шаг 9. service + mapper + scope donate

`internal/donate/internal/service/repository.go` — в интерфейс `Storage` **добавить** `PresignedPutURL(...)` если donate он понадобится; **но** по решению donate картинки сам не грузит (их грузит клиент через media). Поэтому из donate можно убрать использование `UploadObject`/`uploadImage`. Проверить, не остаётся ли `Storage` неиспользуемым — если да, оставить (бакет всё равно создаётся в `fx.go` OnStart-хуке через `BucketExists/CreateBucket/MakeBucketPublic`), эти методы в интерфейсе нужны.

`internal/donate/internal/service/image.go` — `uploadImage` больше не нужен для create/update. Удалить файл (проверить grep на использования). 

`internal/donate/internal/service/service.go`:
- `CreateShopItem`: убрать `s.uploadImage`. Брать `req.ImageUrl`. **Валидировать URL** хелпером (см. ниже) для основной картинки и для каждой `entry.ImageUrl`. Строить модель: если `req.ItemType==ITEM_TYPE_KIT` → `model.NewKitShopItem(...)` + `SetEntries(mapEntries(req.Entries))`, иначе `NewShopItem`. Если `req.HasDiscount` → `item.SetDiscount(req.DiscountPercent)`. Затем `Validate()`.
- `UpdateShopItem`: убрать upload. `image_url` пустой → оставить текущий. Применять `Apply(ShopItemUpdate{...})` с типом/entries/скидкой; если `HasDiscount` false → `ClearDiscount()`. Валидировать URL непустых картинок.
- Хелпер валидации (в service, напр. в новом `image.go` или `validate.go`):
  ```go
  func (s *Service) validateImageURL(u string) error {
  	prefix := fmt.Sprintf("%s/%s/", s.cfg.CdnUrl, bucketName)
  	if u == "" || !strings.HasPrefix(u, prefix) {
  		return status.Error(codes.InvalidArgument, "image_url must be uploaded via media service")
  	}
  	return nil
  }
  ```

`internal/donate/internal/service/mapper.go` (`MapperImpl`, вручную):
- `ToShopItemProto`: маппить `ItemType` (string→enum), `Entries` (`[]model.KitEntry`→`[]*donatev1.KitEntry`), `HasDiscount`, `DiscountPercent`, `EffectivePrice: s.EffectivePrice()`.
  - хелперы: `itemTypeToProto(model.ItemType) donatev1.ItemType` и обратный `itemTypeFromProto`.
- `ToPurchaseProto`: добавить `BasePrice: p.BasePrice`, `DiscountPercent: p.DiscountPercent`.
- Добавить экспортируемые/внутренние конвертеры `KitEntry` model↔proto (proto→model используется в service при создании).

`internal/donate/internal/service/scope.go` — без изменений (CreateShopItem/UpdateShopItem уже под `donate:shop:create`/`update`). Скидка задаётся через Update, отдельный scope не нужен.

`go generate ./...` (на случай если затронуты goverter-домены; donate-mapper ручной, но прогнать для media/kit безопасно).

### Шаг 10. Вывод kit из обращения

`main.go`:
- убрать строку `kit.App,` из списка fx-модулей.
- убрать импорт `"github.com/lasthearth/vsservice/internal/kit"`.
- **Важно:** `internal/server/app.go` и `server.go` всё ещё ссылаются на `kitv1.KitServiceServer` (поле `KitV1` в `Opts`, регистрация). Если убрать `kit.App`, провайдера `KitServiceServer` не будет → fx упадёт. Варианты:
  - (A) Оставить регистрацию kit в server, но без модуля провайдер отсутствует → НЕ годится.
  - (B) Убрать из `server/app.go` (`Opts.KitV1`, поле, `New`) и `server/server.go` (gRPC + gateway регистрацию kit). Это вывод из обращения целиком. **Выбрать (B).** Код домена `internal/kit/**` и `proto/kit`/`gen/kit` остаются нетронутыми — просто не подключены.

Проверить grep, что больше никто не зависит от `kitv1` провайдера через fx.

### Шаг 11. Тесты модели

`internal/donate/internal/model/shop_item_test.go` (дополнить, стиль существующих):
- `EffectivePrice`: без скидки = Price; 0% = Price; 50% от 100 = 50; 100% → clamp 1; percent>0 но цена 1 → ≥1.
- `Validate`: kit с пустыми entries → ошибка; entry quantity<=0 → ошибка; percent 101 → ошибка.
- `SetDiscount` границы (0,100,101,-1); `ClearDiscount` сбрасывает.

`internal/donate/internal/model/purchase_test.go`:
- `NewPurchase` пишет `PricePaid`, `BasePrice`, `DiscountPercent`.

---

## Верификация (обязательно, привести реальный вывод)

```bash
buf generate
go generate ./...
CGO_ENABLED=1 go build -o /tmp/vsservice ./main.go
go test ./internal/donate/... ./internal/media/...
```

Всё должно проходить. Если не собирается/падает — чинить, не оставлять сломанным. В отчёте: путь к плану, список созданных/изменённых файлов по доменам, реальный вывод сборки и тестов, и явный список недоделанного (если есть).

## Миграция данных

Mongo-миграция не нужна (lazy): старые `ShopItem`-документы без `type` → `shopItemFromDTO` дефолтит `item`; без `has_discount`/`discount_percent`/`entries` → Go zero-value (false/0/nil). Старые `Purchase` без `base_price`/`discount_percent` → 0 (исторические записи, отображать как «без скидки»).

## Замечания по безопасности

- Ключ S3-объекта генерит сервер (`uuid.webp`) — клиент имя не выбирает.
- `purpose`→bucket whitelist на стороне media — нельзя presign в произвольный бакет.
- donate валидирует, что каждый `image_url` начинается с `{CdnUrl}/donate-shop/` — защита от инъекции внешних URL.
- presign expiry короткий (15 мин).
