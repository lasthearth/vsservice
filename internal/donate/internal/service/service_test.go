package service_test

import (
	"context"
	"testing"
	"time"

	donatev1 "github.com/lasthearth/vsservice/gen/donate/v1"
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
// so any other call would panic and say so.
type updateItemRepo struct {
	service.DonateRepository
	item *model.ShopItem
}

func (r *updateItemRepo) UpdateShopItem(
	ctx context.Context,
	_ string,
	updateFn func(context.Context, *model.ShopItem) (*model.ShopItem, error),
) (*model.ShopItem, error) {
	return updateFn(ctx, r.item)
}

func newService(t *testing.T, repo service.DonateRepository) *service.Service {
	t.Helper()
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}
	return service.New(service.Opts{
		Repo:     repo,
		Logger:   l,
		Mapper:   &sermapper.MapperImpl{},
		MediaURL: mediaurl.New(config.Config{}),
	})
}

// A validation failure raised inside the repository's update closure must reach
// the client as InvalidArgument. It used to be a gRPC status, which no
// isDomainError branch matched, so it fell through to Internal.
func TestUpdateShopItemRejectsAnInvalidDiscountWithInvalidArgument(t *testing.T) {
	item := model.NewShopItem("code", "Sword", "", "", 100)
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
