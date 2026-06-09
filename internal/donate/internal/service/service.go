package service

import (
	"context"
	"errors"

	donatev1 "github.com/lasthearth/vsservice/gen/donate/v1"
	"github.com/lasthearth/vsservice/internal/donate/internal/ierror"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
	pkgerr "github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Service) AddCoins(ctx context.Context, req *donatev1.AddCoinsRequest) (*donatev1.AddCoinsResponse, error) {
	l := s.log.With(zap.String("method", "AddCoins"), zap.String("player_id", req.PlayerId))

	if req.Amount <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}
	if req.PlayerName == "" {
		return nil, status.Error(codes.InvalidArgument, "player_name is required")
	}

	newCoins, err := s.repo.AddCoinsToWallet(ctx, req.PlayerId, req.PlayerName, req.Amount)
	if err != nil {
		l.Error("failed to add coins", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to add coins")
	}

	tx := model.NewCreditTransaction(req.PlayerId, req.Amount, req.Reason)
	if _, err := s.repo.CreateTransaction(ctx, tx); err != nil {
		l.Error("failed to record transaction", zap.Error(err))
	}

	l.Info("coins added", zap.Int64("amount", req.Amount), zap.Int64("new_coins", newCoins))
	return &donatev1.AddCoinsResponse{Coins: newCoins}, nil
}

func (s *Service) DeductCoins(ctx context.Context, req *donatev1.DeductCoinsRequest) (*donatev1.DeductCoinsResponse, error) {
	l := s.log.With(zap.String("method", "DeductCoins"), zap.String("player_id", req.PlayerId))

	if req.Amount <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}

	var newCoins int64
	err := s.repo.UpdateWallet(ctx, req.PlayerId, func(ctx context.Context, w *model.Wallet) (*model.Wallet, error) {
		if err := w.Withdraw(req.Amount); err != nil {
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

	tx := model.NewDebitTransaction(req.PlayerId, req.Amount, req.Reason)
	if _, err := s.repo.CreateTransaction(ctx, tx); err != nil {
		l.Error("failed to record transaction", zap.Error(err))
	}

	l.Info("coins deducted", zap.Int64("amount", req.Amount), zap.Int64("new_coins", newCoins))
	return &donatev1.DeductCoinsResponse{Coins: newCoins}, nil
}

func (s *Service) CreateShopItem(ctx context.Context, req *donatev1.CreateShopItemRequest) (*donatev1.CreateShopItemResponse, error) {
	l := s.log.With(zap.String("method", "CreateShopItem"))

	imageURL, err := s.uploadImage(ctx, req.Image)
	if err != nil {
		l.Error("failed to upload image", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to upload image")
	}

	item := model.NewShopItem(req.Code, req.Name, req.Description, imageURL, req.Price)
	if err := item.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	created, err := s.repo.CreateShopItem(ctx, item)
	if err != nil {
		l.Error("failed to create shop item", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to create shop item")
	}

	return &donatev1.CreateShopItemResponse{Item: s.mapper.ToShopItemProto(created)}, nil
}

func (s *Service) UpdateShopItem(ctx context.Context, req *donatev1.UpdateShopItemRequest) (*donatev1.UpdateShopItemResponse, error) {
	l := s.log.With(zap.String("method", "UpdateShopItem"), zap.String("id", req.Id))

	var newImageURL string
	if len(req.Image) > 0 {
		url, err := s.uploadImage(ctx, req.Image)
		if err != nil {
			l.Error("failed to upload image", zap.Error(err))
			return nil, status.Error(codes.Internal, "failed to upload image")
		}
		newImageURL = url
	}

	updated, err := s.repo.UpdateShopItem(ctx, req.Id, func(_ context.Context, item *model.ShopItem) (*model.ShopItem, error) {
		imageURL := item.ImageURL
		if newImageURL != "" {
			imageURL = newImageURL
		}
		item.Update(req.Code, req.Name, req.Description, imageURL, req.Price, req.IsAvailable)
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

	return &donatev1.UpdateShopItemResponse{Item: s.mapper.ToShopItemProto(updated)}, nil
}

func (s *Service) DeleteShopItem(ctx context.Context, req *donatev1.DeleteShopItemRequest) (*donatev1.DeleteShopItemResponse, error) {
	l := s.log.With(zap.String("method", "DeleteShopItem"), zap.String("id", req.Id))

	if err := s.repo.DeleteShopItem(ctx, req.Id); err != nil {
		if isDomainError(err, codes.NotFound) {
			return nil, status.Error(codes.NotFound, "shop item not found")
		}
		l.Error("failed to delete shop item", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to delete shop item")
	}

	return &donatev1.DeleteShopItemResponse{}, nil
}

func (s *Service) Refund(ctx context.Context, req *donatev1.RefundRequest) (*donatev1.RefundResponse, error) {
	l := s.log.With(zap.String("method", "Refund"), zap.String("purchase_id", req.PurchaseId))

	purchase, err := s.repo.Refund(ctx, req.PurchaseId, req.Reason)
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
	l := s.log.With(zap.String("method", "ListTransactions"), zap.String("player_id", req.PlayerId))

	txs, err := s.repo.ListTransactionsByPlayerID(ctx, req.PlayerId)
	if err != nil {
		l.Error("failed to list transactions", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list transactions")
	}

	return &donatev1.ListTransactionsResponse{Transactions: s.mapper.ToTransactionsProto(txs)}, nil
}

func (s *Service) AdminListPendingPurchases(ctx context.Context, req *donatev1.AdminListPendingPurchasesRequest) (*donatev1.AdminListPendingPurchasesResponse, error) {
	l := s.log.With(zap.String("method", "AdminListPendingPurchases"))

	purchases, next, err := s.repo.ListPendingPurchases(ctx, req.PageToken, req.Limit)
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
	l := s.log.With(zap.String("method", "MarkPurchaseIssued"), zap.String("purchase_id", req.PurchaseId))

	adminID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	purchase, err := s.repo.MarkPurchaseIssued(ctx, req.PurchaseId, adminID)
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
	l := s.log.With(zap.String("method", "AdminListPurchases"), zap.String("player_id", req.PlayerId))

	purchases, err := s.repo.ListPurchasesByPlayerID(ctx, req.PlayerId)
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

	return &donatev1.ListShopItemsResponse{Items: s.mapper.ToShopItemsProto(items)}, nil
}

func (s *Service) BuyItem(ctx context.Context, req *donatev1.BuyItemRequest) (*donatev1.BuyItemResponse, error) {
	l := s.log.With(zap.String("method", "BuyItem"), zap.String("item_id", req.ItemId))

	playerID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	purchase, err := s.repo.BuyItem(ctx, playerID, req.ItemId)
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
