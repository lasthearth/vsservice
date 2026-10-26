package service

import (
	"context"
	"errors"
	"time"

	donatev1 "github.com/lasthearth/vsservice/gen/donate/v1"
	"github.com/lasthearth/vsservice/internal/donate/internal/goverter"
	"github.com/lasthearth/vsservice/internal/donate/internal/ierror"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
	pkgerr "github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Service) AddCoins(ctx context.Context, req *donatev1.AddCoinsRequest) (*donatev1.AddCoinsResponse, error) {
	l := s.log.With(zap.String("method", "AddCoins"), zap.String("player_id", req.GetPlayerId()))

	if req.GetAmount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}
	if req.GetPlayerName() == "" {
		return nil, status.Error(codes.InvalidArgument, "player_name is required")
	}

	newCoins, err := s.repo.AddCoinsToWallet(ctx, req.GetPlayerId(), req.GetPlayerName(), req.GetAmount())
	if err != nil {
		l.Error("failed to add coins", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to add coins")
	}

	tx := model.NewCreditTransaction(req.GetPlayerId(), req.GetAmount(), req.GetReason())
	if _, err := s.repo.CreateTransaction(ctx, tx); err != nil {
		l.Error("failed to record transaction", zap.Error(err))
	}

	l.Info("coins added", zap.Int64("amount", req.GetAmount()), zap.Int64("new_coins", newCoins))
	return &donatev1.AddCoinsResponse{Coins: newCoins}, nil
}

func (s *Service) DeductCoins(ctx context.Context, req *donatev1.DeductCoinsRequest) (*donatev1.DeductCoinsResponse, error) {
	l := s.log.With(zap.String("method", "DeductCoins"), zap.String("player_id", req.GetPlayerId()))

	if req.GetAmount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}

	var newCoins int64
	err := s.repo.UpdateWallet(ctx, req.GetPlayerId(), func(ctx context.Context, w *model.Wallet) (*model.Wallet, error) {
		if err := w.Withdraw(req.GetAmount()); err != nil {
			return nil, ierror.ErrInsufficientFunds
		}
		newCoins = w.Coins
		return w, nil
	})
	if err != nil {
		if isDomainError(err, codes.FailedPrecondition) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		if isDomainError(err, codes.NotFound) {
			return nil, status.Error(codes.NotFound, "wallet not found")
		}
		l.Error("failed to deduct coins", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to deduct coins")
	}

	tx := model.NewDebitTransaction(req.GetPlayerId(), req.GetAmount(), req.GetReason())
	if _, err := s.repo.CreateTransaction(ctx, tx); err != nil {
		l.Error("failed to record transaction", zap.Error(err))
	}

	l.Info("coins deducted", zap.Int64("amount", req.GetAmount()), zap.Int64("new_coins", newCoins))
	return &donatev1.DeductCoinsResponse{Coins: newCoins}, nil
}

func (s *Service) CreateShopItem(ctx context.Context, req *donatev1.CreateShopItemRequest) (*donatev1.CreateShopItemResponse, error) {
	l := s.log.With(zap.String("method", "CreateShopItem"))

	// Donate sells captured kits only: the code must name a kit the game
	// captured, because the code IS the delivery key (the purchase composes a
	// kit-mail from it). A single-item product is a kit of one item, captured
	// the same way — the hand-typed item code path is closed.
	if req.GetItemType() != donatev1.ItemType_ITEM_TYPE_KIT {
		return nil, status.Error(
			codes.InvalidArgument,
			"item-type shop entries are disabled: donate sells captured kits only; a single-item product is a kit of one item",
		)
	}

	summary, err := s.kitLookup.Get(ctx, req.GetCode())
	if err != nil {
		if isDomainError(err, codes.NotFound) {
			return nil, status.Error(codes.NotFound, "kit code not captured")
		}
		l.Error("failed to look up kit", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to create shop item")
	}

	if err := s.validateImageURL(req.GetImageUrl()); err != nil {
		return nil, err
	}

	// req.Entries is ignored: a kit entry's витрина is derived from the
	// captured kit's items at read time, so a hand-typed display list can
	// never drift from what the purchase actually delivers.
	item := model.NewKitShopItem(req.GetCode(), req.GetName(), req.GetDescription(), req.GetImageUrl(), req.GetPrice())

	if req.GetHasDiscount() {
		if err := item.SetDiscount(req.GetDiscountPercent()); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	item.SetPrivileges(protoPrivilegesToModel(req.GetPrivileges()))
	item.SetDiscountWindow(
		goverter.TimestampToTimePtr(req.GetDiscountStartsAt()),
		goverter.TimestampToTimePtr(req.GetDiscountEndsAt()),
	)

	if err := item.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	created, err := s.repo.CreateShopItem(ctx, item)
	if err != nil {
		// A code another entry already holds, hidden entries included: the
		// repository reports it as a domain error and the caller gets 409 over
		// the gateway instead of a 500 that says nothing about the code.
		if isDomainError(err, codes.AlreadyExists) {
			return nil, status.Error(codes.AlreadyExists, err.Error())
		}
		l.Error("failed to create shop item", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to create shop item")
	}

	pb := s.mapper.ToShopItemProto(created)
	s.fillNowFields(pb, created)
	applyKitSummary(pb, summary)
	return &donatev1.CreateShopItemResponse{Item: pb}, nil
}

func (s *Service) UpdateShopItem(ctx context.Context, req *donatev1.UpdateShopItemRequest) (*donatev1.UpdateShopItemResponse, error) {
	l := s.log.With(zap.String("method", "UpdateShopItem"), zap.String("id", req.GetId()))

	// Kits only, same as CreateShopItem: an explicit item type is refused, and
	// a request that leaves the type unspecified must still land on a kit — a
	// legacy item entry cannot be re-saved without being switched to a kit.
	if req.GetItemType() == donatev1.ItemType_ITEM_TYPE_ITEM {
		return nil, status.Error(
			codes.InvalidArgument,
			"item-type shop entries are disabled: donate sells captured kits only; a single-item product is a kit of one item",
		)
	}

	// A changed code names a different captured kit: validate it the same way
	// the create path does, before any write happens.
	var summary KitSummary
	haveSummary := false
	if code := req.GetCode(); code != "" {
		var err error
		summary, err = s.kitLookup.Get(ctx, code)
		if err != nil {
			if isDomainError(err, codes.NotFound) {
				return nil, status.Error(codes.NotFound, "kit code not captured")
			}
			l.Error("failed to look up kit", zap.Error(err))
			return nil, status.Error(codes.Internal, "failed to update shop item")
		}
		haveSummary = true
	}

	if req.GetImageUrl() != "" {
		if err := s.validateImageURL(req.GetImageUrl()); err != nil {
			return nil, err
		}
	}

	// req.Entries is ignored, same as on create: the витрина of a kit entry is
	// derived from the captured kit's items at read time. Passing nil below
	// also wipes any legacy stored entries on this update — the drift dies
	// with the first re-save.

	updated, err := s.repo.UpdateShopItem(ctx, req.GetId(), func(_ context.Context, item *model.ShopItem) (*model.ShopItem, error) {
		imageURL := item.ImageURL
		if req.GetImageUrl() != "" {
			imageURL = req.GetImageUrl()
		}

		itemType := model.ItemType(itemTypeFromProto(req.GetItemType()))
		if itemType == "" {
			itemType = item.Type
		}
		if itemType != model.ItemTypeKit {
			return nil, pkgerr.InvalidArgument(
				"item-type shop entries are disabled: donate sells captured kits only; a single-item product is a kit of one item",
			)
		}

		u := model.ShopItemUpdate{
			Code:             req.GetCode(),
			Name:             req.GetName(),
			Description:      req.GetDescription(),
			ImageURL:         imageURL,
			Price:            req.GetPrice(),
			IsAvailable:      req.GetIsAvailable(),
			Type:             itemType,
			HasDiscount:      req.GetHasDiscount(),
			DiscountPercent:  req.GetDiscountPercent(),
			Privileges:       protoPrivilegesToModel(req.GetPrivileges()),
			DiscountStartsAt: goverter.TimestampToTimePtr(req.GetDiscountStartsAt()),
			DiscountEndsAt:   goverter.TimestampToTimePtr(req.GetDiscountEndsAt()),
		}
		item.Apply(u)

		if !req.GetHasDiscount() {
			item.ClearDiscount()
			item.SetDiscountWindow(nil, nil)
		}

		if err := item.Validate(); err != nil {
			// A domain error, not a gRPC status: the closure runs inside the
			// repository's update cycle, and isDomainError below is what maps it.
			return nil, pkgerr.InvalidArgument(err.Error())
		}
		return item, nil
	})
	if err != nil {
		if isDomainError(err, codes.NotFound) {
			return nil, status.Error(codes.NotFound, "shop item not found")
		}
		if isDomainError(err, codes.InvalidArgument) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		// Moving the entry onto a code another entry holds, hidden ones
		// included: the same unique index the create path hits, so the same 409.
		if isDomainError(err, codes.AlreadyExists) {
			return nil, status.Error(codes.AlreadyExists, err.Error())
		}
		l.Error("failed to update shop item", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to update shop item")
	}

	pb := s.mapper.ToShopItemProto(updated)
	s.fillNowFields(pb, updated)
	if !haveSummary {
		// The code did not change: fetch the kit's summary for the response.
		if sum, err := s.kitLookup.Get(ctx, updated.Code); err == nil {
			summary = sum
		}
	}
	applyKitSummary(pb, summary)
	return &donatev1.UpdateShopItemResponse{Item: pb}, nil
}

func (s *Service) DeleteShopItem(ctx context.Context, req *donatev1.DeleteShopItemRequest) (*donatev1.DeleteShopItemResponse, error) {
	l := s.log.With(zap.String("method", "DeleteShopItem"), zap.String("id", req.GetId()))

	if err := s.repo.DeleteShopItem(ctx, req.GetId()); err != nil {
		if isDomainError(err, codes.NotFound) {
			return nil, status.Error(codes.NotFound, "shop item not found")
		}
		l.Error("failed to delete shop item", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to delete shop item")
	}

	return &donatev1.DeleteShopItemResponse{}, nil
}

func (s *Service) Refund(ctx context.Context, req *donatev1.RefundRequest) (*donatev1.RefundResponse, error) {
	l := s.log.With(zap.String("method", "Refund"), zap.String("purchase_id", req.GetPurchaseId()))

	purchase, err := s.purchases.Refund(ctx, req.GetPurchaseId(), req.GetReason())
	if err != nil {
		if isDomainError(err, codes.NotFound) {
			return nil, status.Error(codes.NotFound, "purchase not found")
		}
		if isDomainError(err, codes.FailedPrecondition) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		l.Error("failed to refund purchase", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to refund purchase")
	}

	return &donatev1.RefundResponse{Purchase: s.mapper.ToPurchaseProto(purchase)}, nil
}

func (s *Service) ListTransactions(ctx context.Context, req *donatev1.ListTransactionsRequest) (*donatev1.ListTransactionsResponse, error) {
	l := s.log.With(zap.String("method", "ListTransactions"), zap.String("player_id", req.GetPlayerId()))

	txs, err := s.repo.ListTransactionsByPlayerID(ctx, req.GetPlayerId())
	if err != nil {
		l.Error("failed to list transactions", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list transactions")
	}

	return &donatev1.ListTransactionsResponse{Transactions: s.mapper.ToTransactionsProto(txs)}, nil
}

func (s *Service) AdminListPendingPurchases(ctx context.Context, req *donatev1.AdminListPendingPurchasesRequest) (*donatev1.AdminListPendingPurchasesResponse, error) {
	l := s.log.With(zap.String("method", "AdminListPendingPurchases"))

	purchases, next, err := s.repo.ListPendingPurchases(ctx, req.GetPageToken(), req.GetLimit())
	if err != nil {
		l.Error("failed to list pending purchases", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list pending purchases")
	}

	return &donatev1.AdminListPendingPurchasesResponse{
		Purchases:     s.mapper.ToPurchasesProto(purchases),
		NextPageToken: next,
	}, nil
}

func (s *Service) AdminListAllPurchases(ctx context.Context, req *donatev1.AdminListAllPurchasesRequest) (*donatev1.AdminListAllPurchasesResponse, error) {
	l := s.log.With(zap.String("method", "AdminListAllPurchases"))

	purchases, next, err := s.repo.ListAllPurchases(ctx, req.GetPageToken(), req.GetLimit())
	if err != nil {
		l.Error("failed to list all purchases", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list all purchases")
	}

	return &donatev1.AdminListAllPurchasesResponse{
		Purchases:     s.mapper.ToPurchasesProto(purchases),
		NextPageToken: next,
	}, nil
}

func (s *Service) MarkPurchaseIssued(ctx context.Context, req *donatev1.MarkPurchaseIssuedRequest) (*donatev1.MarkPurchaseIssuedResponse, error) {
	l := s.log.With(zap.String("method", "MarkPurchaseIssued"), zap.String("purchase_id", req.GetPurchaseId()))

	adminID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	purchase, err := s.purchases.MarkIssued(ctx, req.GetPurchaseId(), adminID)
	if err != nil {
		if isDomainError(err, codes.NotFound) {
			return nil, status.Error(codes.NotFound, "purchase not found")
		}
		if isDomainError(err, codes.FailedPrecondition) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		l.Error("failed to mark purchase issued", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to mark purchase issued")
	}

	return &donatev1.MarkPurchaseIssuedResponse{Purchase: s.mapper.ToPurchaseProto(purchase)}, nil
}

func (s *Service) AdminListPurchases(ctx context.Context, req *donatev1.AdminListPurchasesRequest) (*donatev1.AdminListPurchasesResponse, error) {
	l := s.log.With(zap.String("method", "AdminListPurchases"), zap.String("player_id", req.GetPlayerId()))

	purchases, err := s.repo.ListPurchasesByPlayerID(ctx, req.GetPlayerId())
	if err != nil {
		l.Error("failed to list purchases", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list purchases")
	}

	return &donatev1.AdminListPurchasesResponse{Purchases: s.mapper.ToPurchasesProto(purchases)}, nil
}

func (s *Service) AdminGetPlayerBalance(ctx context.Context, req *donatev1.AdminGetPlayerBalanceRequest) (*donatev1.AdminGetPlayerBalanceResponse, error) {
	l := s.log.With(zap.String("method", "AdminGetPlayerBalance"), zap.String("player_id", req.GetPlayerId()))

	wallet, err := s.repo.GetWalletByPlayerID(ctx, req.GetPlayerId())
	if err != nil {
		if isDomainError(err, codes.NotFound) {
			return &donatev1.AdminGetPlayerBalanceResponse{PlayerId: req.GetPlayerId(), Coins: 0}, nil
		}
		l.Error("failed to get balance", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get balance")
	}

	return &donatev1.AdminGetPlayerBalanceResponse{
		PlayerId:   wallet.PlayerID,
		PlayerName: wallet.PlayerName,
		Coins:      wallet.Coins,
	}, nil
}

func (s *Service) GetMyBalance(ctx context.Context, _ *donatev1.GetMyBalanceRequest) (*donatev1.GetMyBalanceResponse, error) {
	l := s.log.With(zap.String("method", "GetMyBalance"))

	playerID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	wallet, err := s.repo.GetWalletByPlayerID(ctx, playerID)
	if err != nil {
		if isDomainError(err, codes.NotFound) {
			return &donatev1.GetMyBalanceResponse{Coins: 0}, nil
		}
		l.Error("failed to get balance", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get balance")
	}

	return &donatev1.GetMyBalanceResponse{Coins: wallet.Coins}, nil
}

func (s *Service) ListShopItems(ctx context.Context, _ *donatev1.ListShopItemsRequest) (*donatev1.ListShopItemsResponse, error) {
	l := s.log.With(zap.String("method", "ListShopItems"))

	items, err := s.repo.ListShopItems(ctx, true)
	if err != nil {
		l.Error("failed to list shop items", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list shop items")
	}

	return &donatev1.ListShopItemsResponse{Items: s.shopItemsProto(ctx, l, items)}, nil
}

// AdminListAllShopItems answers the admin panel: every shop entry, the hidden
// ones included. The public list filters on is_available, so an entry the site
// hid was invisible in the one panel that can publish it again.
//
// Scope: donate:shop:update (scope.go). There is no donate:shop:read, and the
// token that edits the shop in the panel carries update, so the admin read of
// the same resource shares it.
func (s *Service) AdminListAllShopItems(ctx context.Context, _ *donatev1.AdminListAllShopItemsRequest) (*donatev1.AdminListAllShopItemsResponse, error) {
	l := s.log.With(zap.String("method", "AdminListAllShopItems"))

	items, err := s.repo.ListShopItems(ctx, false)
	if err != nil {
		l.Error("failed to list shop items", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list shop items")
	}

	return &donatev1.AdminListAllShopItemsResponse{Items: s.shopItemsProto(ctx, l, items)}, nil
}

// shopItemsProto maps shop items to proto, stamps the two computed fields from
// one clock and joins the kits in one catalog read — category AND витрина:
// a kit entry's entries are derived from the captured kit's items (game code,
// stack size, game-rendered icon), never from a stored display list. Both
// shop lists go through it so they answer the same shape.
//
// Fail-open on the join: a kit lookup outage leaves the categories empty and
// the stored legacy entries in place rather than 500-ing the list — the entry
// itself stays sellable.
func (s *Service) shopItemsProto(ctx context.Context, l logger.Logger, items []*model.ShopItem) []*donatev1.ShopItem {
	catalog := map[string]KitSummary{}
	if cat, err := s.kitLookup.Catalog(ctx); err == nil {
		catalog = cat
	} else {
		l.Warn("failed to join kit catalog; shop items list without categories and derived entries", zap.Error(err))
	}

	now := time.Now()
	pbItems := s.mapper.ToShopItemsProto(items)
	for i, m := range items {
		s.fillNowFieldsAt(pbItems[i], m, now)
		if m.Type == model.ItemTypeKit {
			if summary, ok := catalog[m.Code]; ok {
				applyKitSummary(pbItems[i], summary)
			}
			// No captured kit (yet): the stored legacy entries stay as they
			// are, so a hidden pre-capture entry still renders for the admin.
		}
	}
	return pbItems
}

// applyKitSummary stamps the kit-derived fields onto a wire shop item: the
// category and the витрина — one entry per captured kit item, named by its
// game code, sized by its stack, pictured by the icon uploaded at capture.
func applyKitSummary(pb *donatev1.ShopItem, summary KitSummary) {
	pb.Category = summary.Category
	entries := make([]*donatev1.KitEntry, len(summary.Items))
	for i, it := range summary.Items {
		entries[i] = &donatev1.KitEntry{
			Name:        it.GameCode,
			Description: "",
			ImageUrl:    it.ImageURL,
			Quantity:    it.Quantity,
		}
	}
	pb.Entries = entries
}

func (s *Service) BuyItem(ctx context.Context, req *donatev1.BuyItemRequest) (*donatev1.BuyItemResponse, error) {
	l := s.log.With(zap.String("method", "BuyItem"), zap.String("item_id", req.GetItemId()))

	playerID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	purchase, err := s.purchases.Buy(ctx, playerID, req.GetItemId())
	if err != nil {
		if isDomainError(err, codes.NotFound) {
			return nil, status.Error(codes.NotFound, "item not found or unavailable")
		}
		if isDomainError(err, codes.FailedPrecondition) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		l.Error("failed to buy item", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to buy item")
	}

	return &donatev1.BuyItemResponse{Purchase: s.mapper.ToPurchaseProto(purchase)}, nil
}

func (s *Service) ListMyPurchases(ctx context.Context, _ *donatev1.ListMyPurchasesRequest) (*donatev1.ListMyPurchasesResponse, error) {
	l := s.log.With(zap.String("method", "ListMyPurchases"))

	playerID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	purchases, err := s.repo.ListPurchasesByPlayerID(ctx, playerID)
	if err != nil {
		l.Error("failed to list purchases", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list purchases")
	}

	return &donatev1.ListMyPurchasesResponse{Purchases: s.mapper.ToPurchasesProto(purchases)}, nil
}

func (s *Service) ListWallets(ctx context.Context, req *donatev1.ListWalletsRequest) (*donatev1.ListWalletsResponse, error) {
	l := s.log.With(zap.String("method", "ListWallets"))

	wallets, next, err := s.repo.ListWallets(ctx, req.GetPageToken(), req.GetLimit())
	if err != nil {
		l.Error("failed to list wallets", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list wallets")
	}

	return &donatev1.ListWalletsResponse{
		Wallets:       s.mapper.ToWalletBalancesProto(wallets),
		NextPageToken: next,
	}, nil
}

func isDomainError(err error, code codes.Code) bool {
	var de *pkgerr.DomainError
	return errors.As(err, &de) && de.Code == code
}

// protoPrivilegesToModel converts proto Privilege slice to model Privilege slice.
func protoPrivilegesToModel(ps []*donatev1.Privilege) []model.Privilege {
	result := make([]model.Privilege, len(ps))
	for i, p := range ps {
		result[i] = model.Privilege{
			Text: p.GetText(),
			Icon: p.GetIcon(),
		}
	}
	return result
}

// itemTypeFromProto converts a proto ItemType to the string used in the model.
func itemTypeFromProto(t donatev1.ItemType) string {
	switch t {
	case donatev1.ItemType_ITEM_TYPE_KIT:
		return string(model.ItemTypeKit)
	case donatev1.ItemType_ITEM_TYPE_ITEM:
		return string(model.ItemTypeItem)
	default:
		return ""
	}
}

// fillNowFields stamps DiscountActive and EffectivePrice on pb using time.Now().
func (s *Service) fillNowFields(pb *donatev1.ShopItem, m *model.ShopItem) {
	s.fillNowFieldsAt(pb, m, time.Now())
}

// fillNowFieldsAt stamps DiscountActive and EffectivePrice on pb using a shared now.
func (s *Service) fillNowFieldsAt(pb *donatev1.ShopItem, m *model.ShopItem, now time.Time) {
	pb.DiscountActive = m.DiscountActive(now)
	pb.EffectivePrice = m.EffectivePriceAt(now)
}
