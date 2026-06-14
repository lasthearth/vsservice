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

	if err := s.validateImageURL(req.GetImageUrl()); err != nil {
		return nil, err
	}

	var item *model.ShopItem
	if req.GetItemType() == donatev1.ItemType_ITEM_TYPE_KIT {
		entries, err := protoEntriesToModel(req.GetEntries())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		item = model.NewKitShopItem(req.GetCode(), req.GetName(), req.GetDescription(), req.GetImageUrl(), req.GetPrice(), entries)
	} else {
		item = model.NewShopItem(req.GetCode(), req.GetName(), req.GetDescription(), req.GetImageUrl(), req.GetPrice())
	}

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

	// Validate entry image URLs for kit
	for _, e := range req.GetEntries() {
		if e.GetImageUrl() != "" {
			if err := s.validateImageURL(e.GetImageUrl()); err != nil {
				return nil, err
			}
		}
	}

	if err := item.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	created, err := s.repo.CreateShopItem(ctx, item)
	if err != nil {
		l.Error("failed to create shop item", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to create shop item")
	}

	pb := s.mapper.ToShopItemProto(created)
	s.fillNowFields(pb, created)
	return &donatev1.CreateShopItemResponse{Item: pb}, nil
}

func (s *Service) UpdateShopItem(ctx context.Context, req *donatev1.UpdateShopItemRequest) (*donatev1.UpdateShopItemResponse, error) {
	l := s.log.With(zap.String("method", "UpdateShopItem"), zap.String("id", req.GetId()))

	if req.GetImageUrl() != "" {
		if err := s.validateImageURL(req.GetImageUrl()); err != nil {
			return nil, err
		}
	}

	// Validate entry image URLs
	for _, e := range req.GetEntries() {
		if e.GetImageUrl() != "" {
			if err := s.validateImageURL(e.GetImageUrl()); err != nil {
				return nil, err
			}
		}
	}

	entries, err := protoEntriesToModel(req.GetEntries())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	updated, err := s.repo.UpdateShopItem(ctx, req.GetId(), func(_ context.Context, item *model.ShopItem) (*model.ShopItem, error) {
		imageURL := item.ImageURL
		if req.GetImageUrl() != "" {
			imageURL = req.GetImageUrl()
		}

		itemType := model.ItemType(itemTypeFromProto(req.GetItemType()))
		if itemType == "" {
			itemType = item.Type
		}

		u := model.ShopItemUpdate{
			Code:             req.GetCode(),
			Name:             req.GetName(),
			Description:      req.GetDescription(),
			ImageURL:         imageURL,
			Price:            req.GetPrice(),
			IsAvailable:      req.GetIsAvailable(),
			Type:             itemType,
			Entries:          entries,
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
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return item, nil
	})
	if err != nil {
		if isDomainError(err, codes.NotFound) {
			return nil, status.Error(codes.NotFound, "shop item not found")
		}
		l.Error("failed to update shop item", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to update shop item")
	}

	pb := s.mapper.ToShopItemProto(updated)
	s.fillNowFields(pb, updated)
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

	purchase, err := s.repo.Refund(ctx, req.GetPurchaseId(), req.GetReason())
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

func (s *Service) MarkPurchaseIssued(ctx context.Context, req *donatev1.MarkPurchaseIssuedRequest) (*donatev1.MarkPurchaseIssuedResponse, error) {
	l := s.log.With(zap.String("method", "MarkPurchaseIssued"), zap.String("purchase_id", req.GetPurchaseId()))

	adminID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	purchase, err := s.repo.MarkPurchaseIssued(ctx, req.GetPurchaseId(), adminID)
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

	now := time.Now()
	pbItems := s.mapper.ToShopItemsProto(items)
	for i, m := range items {
		s.fillNowFieldsAt(pbItems[i], m, now)
	}
	return &donatev1.ListShopItemsResponse{Items: pbItems}, nil
}

func (s *Service) BuyItem(ctx context.Context, req *donatev1.BuyItemRequest) (*donatev1.BuyItemResponse, error) {
	l := s.log.With(zap.String("method", "BuyItem"), zap.String("item_id", req.GetItemId()))

	playerID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	purchase, err := s.repo.BuyItem(ctx, playerID, req.GetItemId())
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

// protoEntriesToModel converts proto KitEntry slice to model KitEntry slice.
func protoEntriesToModel(entries []*donatev1.KitEntry) ([]model.KitEntry, error) {
	result := make([]model.KitEntry, len(entries))
	for i, e := range entries {
		result[i] = model.KitEntry{
			Name:        e.GetName(),
			Description: e.GetDescription(),
			ImageURL:    e.GetImageUrl(),
			Quantity:    e.GetQuantity(),
		}
	}
	return result, nil
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
