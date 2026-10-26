package service_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	donatev1 "github.com/lasthearth/vsservice/gen/donate/v1"
	"github.com/lasthearth/vsservice/internal/donate/internal/ierror"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
	"github.com/lasthearth/vsservice/internal/donate/internal/service"
	"github.com/lasthearth/vsservice/internal/donate/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mediaurl"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// updateItemRepo implements only UpdateShopItem; the embedded interface is nil,
// so any other call would panic and say so. A non-nil err stands for a write
// that failed before the closure's result was persisted, which is where the
// unique index on code rejects a code change.
type updateItemRepo struct {
	service.DonateRepository
	item *model.ShopItem
	err  error
}

func (r *updateItemRepo) UpdateShopItem(
	ctx context.Context,
	_ string,
	updateFn func(context.Context, *model.ShopItem) (*model.ShopItem, error),
) (*model.ShopItem, error) {
	if r.err != nil {
		return nil, r.err
	}
	return updateFn(ctx, r.item)
}

// stubKitLookup stands in for the kitdef read port: every code resolves to a
// known category, so the kit-only validation passes.
type stubKitLookup struct {
	categories map[string]string
}

func (s *stubKitLookup) GetCategory(ctx context.Context, code string) (string, error) {
	if c, ok := s.categories[code]; ok {
		return c, nil
	}
	return "kit", nil
}

func (s *stubKitLookup) Categories(ctx context.Context) (map[string]string, error) {
	return s.categories, nil
}

func newService(t *testing.T, repo service.DonateRepository) *service.Service {
	t.Helper()
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}
	return service.New(service.Opts{
		Repo:   repo,
		Logger: l,
		Mapper: &sermapper.MapperImpl{},
		// A CDN host: the validator rejects every host it was not configured
		// with, and the create path requires an image_url.
		MediaURL: mediaurl.New(config.Config{CdnUrl: "https://cdn.test"}),
		KitLookup: &stubKitLookup{
			categories: map[string]string{"code": "kit", "foodkit": "privilege"},
		},
	})
}

// A validation failure raised inside the repository's update closure must reach
// the client as InvalidArgument. It used to be a gRPC status, which no
// isDomainError branch matched, so it fell through to Internal.
func TestUpdateShopItemRejectsAnInvalidDiscountWithInvalidArgument(t *testing.T) {
	item := model.NewKitShopItem("code", "Sword", "", "", 100, []model.KitEntry{{Name: "Sword", Quantity: 1}})
	svc := newService(t, &updateItemRepo{item: item})

	_, err := svc.UpdateShopItem(context.Background(), &donatev1.UpdateShopItemRequest{
		Id:              "i1",
		Code:            "code",
		Name:            "Sword",
		Price:           100,
		HasDiscount:     true,
		DiscountPercent: 200,
	})
	if err == nil {
		t.Fatal("UpdateShopItem: got nil error, want InvalidArgument")
	}
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Fatalf("code = %s, want InvalidArgument", got)
	}
	if msg := status.Convert(err).Message(); msg != "discount_percent must be between 0 and 100" {
		t.Fatalf("message = %q, want the validation message", msg)
	}
}

// Moving an entry onto a code another entry already holds hits the same unique
// index the create path hits, and a hidden entry holds its code too. It used to
// fall through to 500 "failed to update shop item"; the caller must get the 409
// that names the code as the problem.
func TestUpdateShopItemAnswersAlreadyExistsForATakenCode(t *testing.T) {
	item := model.NewKitShopItem("code", "Sword", "", "", 100, []model.KitEntry{{Name: "Sword", Quantity: 1}})
	svc := newService(t, &updateItemRepo{item: item, err: ierror.ErrShopCodeExists})

	_, err := svc.UpdateShopItem(context.Background(), &donatev1.UpdateShopItemRequest{
		Id:       "i1",
		Code:     "foodkit",
		Name:     "Sword",
		Price:    100,
		ItemType: donatev1.ItemType_ITEM_TYPE_KIT,
		Entries:  []*donatev1.KitEntry{{Name: "Sword", Quantity: 1}},
	})
	if err == nil {
		t.Fatal("UpdateShopItem: got nil error, want AlreadyExists")
	}
	if got := status.Code(err); got != codes.AlreadyExists {
		t.Fatalf("code = %s, want AlreadyExists", got)
	}
	if msg := status.Convert(err).Message(); msg != "shop entry with this code already exists" {
		t.Fatalf("message = %q, want the taken-code message", msg)
	}
	if got := runtime.HTTPStatusFromCode(status.Code(err)); got != http.StatusConflict {
		t.Fatalf("gateway status = %d, want %d", got, http.StatusConflict)
	}
}

// listItemsRepo implements only ListShopItems.
type listItemsRepo struct {
	service.DonateRepository
	items []*model.ShopItem
}

func (r *listItemsRepo) ListShopItems(context.Context, bool) ([]*model.ShopItem, error) {
	return r.items, nil
}

// EffectivePrice and DiscountActive are stamped by the service from one clock,
// not by the mapper. Before, the mapper computed EffectivePrice from its own
// time.Now() and the service overwrote it — correct only by statement order.
func TestListShopItemsStampsTheDiscountedPrice(t *testing.T) {
	item := model.NewShopItem("code", "Sword", "", "", 50)
	if err := item.SetDiscount(40); err != nil {
		t.Fatalf("SetDiscount: %v", err)
	}
	start := time.Now().Add(-time.Hour)
	end := time.Now().Add(time.Hour)
	item.SetDiscountWindow(&start, &end)

	svc := newService(t, &listItemsRepo{items: []*model.ShopItem{item}})

	resp, err := svc.ListShopItems(context.Background(), &donatev1.ListShopItemsRequest{})
	if err != nil {
		t.Fatalf("ListShopItems: %v", err)
	}
	if n := len(resp.GetItems()); n != 1 {
		t.Fatalf("items = %d, want 1", n)
	}
	got := resp.GetItems()[0]
	if got.GetEffectivePrice() != 30 {
		t.Fatalf("effective_price = %d, want 30", got.GetEffectivePrice())
	}
	if !got.GetDiscountActive() {
		t.Fatal("discount_active = false, want true inside the window")
	}
	if got.GetPrice() != 50 {
		t.Fatalf("price = %d, want the undiscounted 50", got.GetPrice())
	}
}

// createItemRepo implements only CreateShopItem and answers with the error it
// was given; the embedded interface is nil, so any other call would panic and
// say so.
type createItemRepo struct {
	service.DonateRepository
	err error
}

func (r *createItemRepo) CreateShopItem(context.Context, *model.ShopItem) (*model.ShopItem, error) {
	return nil, r.err
}

// donate_shop_items has a unique index on code and every one of the 18 entries
// on prod is hidden, so a code that looks free in the shop list can still be
// taken. The repository reports the driver's E11000 as a domain error; it must
// reach the caller as AlreadyExists, which the gateway answers 409, and not as
// the 500 "failed to create shop item" that says nothing about the code.
func TestCreateShopItemAnswersAlreadyExistsForATakenCode(t *testing.T) {
	svc := newService(t, &createItemRepo{err: ierror.ErrShopCodeExists})

	_, err := svc.CreateShopItem(context.Background(), &donatev1.CreateShopItemRequest{
		Name:     "Rations",
		Code:     "foodkit",
		ImageUrl: "https://cdn.test/foodkit.png",
		Price:    50,
		ItemType: donatev1.ItemType_ITEM_TYPE_KIT,
		Entries:  []*donatev1.KitEntry{{Name: "Bread", Quantity: 2}},
	})
	if err == nil {
		t.Fatal("CreateShopItem: got nil error, want AlreadyExists")
	}
	if got := status.Code(err); got != codes.AlreadyExists {
		t.Fatalf("code = %s, want AlreadyExists", got)
	}
	if msg := status.Convert(err).Message(); msg != "shop entry with this code already exists" {
		t.Fatalf("message = %q, want the taken-code message", msg)
	}
	if got := runtime.HTTPStatusFromCode(status.Code(err)); got != http.StatusConflict {
		t.Fatalf("gateway status = %d, want %d", got, http.StatusConflict)
	}
}

// A failure that is not a taken code stays a 500: only the duplicate has an
// answer the caller can act on.
func TestCreateShopItemKeepsInternalForAnUnrelatedFailure(t *testing.T) {
	svc := newService(t, &createItemRepo{err: errors.New("server selection timeout")})

	_, err := svc.CreateShopItem(context.Background(), &donatev1.CreateShopItemRequest{
		Name:     "Rations",
		Code:     "foodkit",
		ImageUrl: "https://cdn.test/foodkit.png",
		Price:    50,
		ItemType: donatev1.ItemType_ITEM_TYPE_KIT,
		Entries:  []*donatev1.KitEntry{{Name: "Bread", Quantity: 2}},
	})
	if got := status.Code(err); got != codes.Internal {
		t.Fatalf("code = %s, want Internal", got)
	}
}

// listAllItemsRepo implements only ListShopItems and records the filter it was
// asked for.
type listAllItemsRepo struct {
	service.DonateRepository
	items         []*model.ShopItem
	availableOnly bool
}

func (r *listAllItemsRepo) ListShopItems(_ context.Context, availableOnly bool) ([]*model.ShopItem, error) {
	r.availableOnly = availableOnly
	return r.items, nil
}

// The public list filters on is_available, so a hidden entry was invisible in
// the one panel that can publish it again. The admin variant must ask the
// repository for every entry and answer them in the same shape the public list
// does: is_available telling the two apart, and the kit category joined.
func TestAdminListAllShopItemsReturnsTheHiddenEntries(t *testing.T) {
	published := model.NewKitShopItem("code", "Sword", "", "", 100, []model.KitEntry{{Name: "Sword", Quantity: 1}})
	// Reconstituted, not constructed: this is what the repository hands the
	// service for a persisted entry the site hid.
	hidden := model.ReconstituteShopItem(
		"64f0a1b2c3d4e5f6a7b8c9d0", "foodkit", "Rations", "", "", 50, false, model.ItemTypeKit,
		[]model.KitEntry{{Name: "Bread", Quantity: 2}},
		false, 0, nil, nil, nil, time.Time{}, time.Time{},
	)

	repo := &listAllItemsRepo{items: []*model.ShopItem{published, hidden}}
	svc := newService(t, repo)

	resp, err := svc.AdminListAllShopItems(context.Background(), &donatev1.AdminListAllShopItemsRequest{})
	if err != nil {
		t.Fatalf("AdminListAllShopItems: %v", err)
	}
	if repo.availableOnly {
		t.Fatal("availableOnly = true, want false: the admin list must not filter")
	}

	items := resp.GetItems()
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if !items[0].GetIsAvailable() {
		t.Fatal("items[0].is_available = false, want the published entry")
	}
	if items[1].GetIsAvailable() {
		t.Fatal("items[1].is_available = true, want the hidden entry")
	}
	if got, want := items[0].GetCategory(), "kit"; got != want {
		t.Fatalf("items[0].category = %q, want %q", got, want)
	}
	if got, want := items[1].GetCategory(), "privilege"; got != want {
		t.Fatalf("items[1].category = %q, want %q", got, want)
	}
}

// The admin list reuses the public list's shaping, so a kit lookup outage still
// leaves an answer instead of a 500: the panel keeps the entries it has to
// publish, only without their category.
func TestAdminListAllShopItemsSurvivesAKitLookupOutage(t *testing.T) {
	hidden := model.ReconstituteShopItem(
		"64f0a1b2c3d4e5f6a7b8c9d0", "foodkit", "Rations", "", "", 50, false, model.ItemTypeKit,
		[]model.KitEntry{{Name: "Bread", Quantity: 2}},
		false, 0, nil, nil, nil, time.Time{}, time.Time{},
	)

	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(service.Opts{
		Repo:      &listAllItemsRepo{items: []*model.ShopItem{hidden}},
		Logger:    l,
		Mapper:    &sermapper.MapperImpl{},
		MediaURL:  mediaurl.New(config.Config{CdnUrl: "https://cdn.test"}),
		KitLookup: failingKitLookup{},
	})

	resp, err := svc.AdminListAllShopItems(context.Background(), &donatev1.AdminListAllShopItemsRequest{})
	if err != nil {
		t.Fatalf("AdminListAllShopItems: %v, want the list without categories", err)
	}
	if n := len(resp.GetItems()); n != 1 {
		t.Fatalf("items = %d, want 1", n)
	}
	if got := resp.GetItems()[0].GetCategory(); got != "" {
		t.Fatalf("category = %q, want empty when the kit lookup is down", got)
	}
}

// failingKitLookup stands for a kitdef outage.
type failingKitLookup struct{}

func (failingKitLookup) GetCategory(context.Context, string) (string, error) {
	return "", errors.New("kitdef is down")
}

func (failingKitLookup) Categories(context.Context) (map[string]string, error) {
	return nil, errors.New("kitdef is down")
}
