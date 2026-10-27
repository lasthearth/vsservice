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
		// The balance is what a coin grant is: the record names the amount it
		// tried to move and the reason it was asked to, so a support ticket can
		// be answered from the log even when the write never landed.
		s.opFor(ctx).Event(opWalletCredited).
			Entity(entityWallet, req.GetPlayerId()).
			Reason(req.GetReason()).
			Field(zap.Int64("amount", req.GetAmount()), zap.String("currency", coins)).
			Error("failed to credit the wallet", err)
		return nil, status.Error(codes.Internal, "failed to add coins")
	}

	tx := model.NewCreditTransaction(req.GetPlayerId(), req.GetAmount(), req.GetReason())
	if _, err := s.repo.CreateTransaction(ctx, tx); err != nil {
		// The ledger row is a separate fact from the credit above: the coins are
		// already on the wallet, so this failure costs a report line, not money.
		l.Error("failed to record transaction", zap.Error(err))
	}

	// The credit answer carries the new balance, so the record states it
	// without a second read of the wallet.
	s.opFor(ctx).Event(opWalletCredited).
		Entity(entityWallet, req.GetPlayerId()).
		Reason(req.GetReason()).
		Field(
			zap.Int64("amount", req.GetAmount()),
			zap.Int64("balance", newCoins),
			zap.String("currency", coins),
		).
		Info("coins credited")
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
			// A rule the player can see and act on, not a failure: the
			// wallet is intact, so the record is a Warn that carries the reason.
			s.opFor(ctx).Event(opWalletDebitRefused).
				Entity(entityWallet, req.GetPlayerId()).
				Reason(err.Error()).
				Field(zap.Int64("amount", req.GetAmount()), zap.String("currency", coins)).
				Warn("debit refused by the wallet rules")
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		if isDomainError(err, codes.NotFound) {
			s.opFor(ctx).Event(opWalletDebitRefused).
				Entity(entityWallet, req.GetPlayerId()).
				Reason("wallet not found").
				Field(zap.Int64("amount", req.GetAmount()), zap.String("currency", coins)).
				Warn("debit refused by the wallet rules")
			return nil, status.Error(codes.NotFound, "wallet not found")
		}
		s.opFor(ctx).Event(opWalletDebited).
			Entity(entityWallet, req.GetPlayerId()).
			Reason(req.GetReason()).
			Field(zap.Int64("amount", req.GetAmount()), zap.String("currency", coins)).
			Error("failed to debit the wallet", err)
		return nil, status.Error(codes.Internal, "failed to deduct coins")
	}

	tx := model.NewDebitTransaction(req.GetPlayerId(), req.GetAmount(), req.GetReason())
	if _, err := s.repo.CreateTransaction(ctx, tx); err != nil {
		// The ledger row is a separate fact from the debit above: the coins are
		// already off the wallet, so this failure costs a report line, not money.
		l.Error("failed to record transaction", zap.Error(err))
	}

	// The withdrawal ran inside the update closure, so newCoins is the balance
	// the write left behind: no second read to state it.
	s.opFor(ctx).Event(opWalletDebited).
		Entity(entityWallet, req.GetPlayerId()).
		Reason(req.GetReason()).
		Field(
			zap.Int64("amount", req.GetAmount()),
			zap.Int64("balance", newCoins),
			zap.String("currency", coins),
		).
		Info("coins debited")
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

	entries := s.kitEntriesFromProto(req.GetEntries())
	item := model.NewKitShopItem(req.GetCode(), req.GetName(), req.GetDescription(), req.GetImageUrl(), req.GetPrice())
	item.SetEntries(entries)
	item.SetSection(req.GetSection())

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

	entries := s.kitEntriesFromProto(req.GetEntries())

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
			Entries:          entries,
			HasDiscount:      req.GetHasDiscount(),
			DiscountPercent:  req.GetDiscountPercent(),
			Privileges:       protoPrivilegesToModel(req.GetPrivileges()),
			DiscountStartsAt: goverter.TimestampToTimePtr(req.GetDiscountStartsAt()),
			DiscountEndsAt:   goverter.TimestampToTimePtr(req.GetDiscountEndsAt()),
			Section:          req.GetSection(),
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
	purchase, err := s.purchases.Refund(ctx, req.GetPurchaseId(), req.GetReason())
	if err != nil {
		if isDomainError(err, codes.NotFound) {
			s.opFor(ctx).Event(opPurchaseRefundRefused).
				Entity(entityPurchase, req.GetPurchaseId()).
				Reason("purchase not found").
				Warn("refund refused: no such purchase")
			return nil, status.Error(codes.NotFound, "purchase not found")
		}
		if isDomainError(err, codes.FailedPrecondition) {
			// "Already refunded" is the answer, not a fault: the guard is what
			// keeps a retry from paying twice, so the refusal is the fact worth
			// keeping.
			s.opFor(ctx).Event(opPurchaseRefundRefused).
				Entity(entityPurchase, req.GetPurchaseId()).
				Reason(err.Error()).
				Warn("refund refused by the purchase rules")
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		s.opFor(ctx).Event(opPurchaseRefunded).
			Entity(entityPurchase, req.GetPurchaseId()).
			Reason(req.GetReason()).
			Error("failed to refund the purchase", err)
		return nil, status.Error(codes.Internal, "failed to refund purchase")
	}

	// The refunded purchase carries what was paid, so the record states the
	// money that went back without reading the wallet again.
	s.opFor(ctx).Event(opPurchaseRefunded).
		Entity(entityPurchase, purchase.Id).
		Reason(req.GetReason()).
		Transition(string(model.PurchaseStatusActive), string(purchase.Status)).
		Field(
			zap.String("item_id", purchase.ItemID),
			zap.Int64("amount", purchase.PricePaid),
			zap.String("currency", coins),
		).
		Info("purchase refunded")

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
	adminID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	purchase, err := s.purchases.MarkIssued(ctx, req.GetPurchaseId(), adminID)
	if err != nil {
		if isDomainError(err, codes.NotFound) {
			s.opFor(ctx).Event(opPurchaseIssueRefused).
				Entity(entityPurchase, req.GetPurchaseId()).
				Reason("purchase not found").
				Warn("issue refused: no such purchase")
			return nil, status.Error(codes.NotFound, "purchase not found")
		}
		if isDomainError(err, codes.FailedPrecondition) {
			s.opFor(ctx).Event(opPurchaseIssueRefused).
				Entity(entityPurchase, req.GetPurchaseId()).
				Reason(err.Error()).
				Warn("issue refused by the purchase rules")
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		s.opFor(ctx).Event(opPurchaseIssued).
			Entity(entityPurchase, req.GetPurchaseId()).
			Error("failed to mark purchase issued", err)
		return nil, status.Error(codes.Internal, "failed to mark purchase issued")
	}

	// The admin who delivered the kit is the fact a later dispute asks about,
	// and opFor already carries them as actor_id from the token the handler
	// read above.
	s.opFor(ctx).Event(opPurchaseIssued).
		Entity(entityPurchase, purchase.Id).
		Transition(string(purchase.Status), "issued").
		Field(zap.String("item_id", purchase.ItemID)).
		Info("purchase issued")

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
// one clock and joins the kits in one catalog read: the category, and the
// витрина where the admin stored none. Both shop lists go through it so they
// answer the same shape.
//
// Fail-open on the join: a kit lookup outage leaves the categories empty and
// the stored entries as they are rather than 500-ing the list — the entry
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
// category and the витрина. The витрина is built from the captured kit, so the
// contents are always what the purchase delivers; the stored rows only decorate
// them (see displayKitEntries).
func applyKitSummary(pb *donatev1.ShopItem, summary KitSummary) {
	pb.Category = summary.Category
	pb.Entries = displayKitEntries(pb.GetEntries(), summary.Items)
}

// displayKitEntries builds the витрина of a kit: one row per captured kit item,
// in the order of the stored rows. A stored row names its captured item by
// game_code (the first still-unmatched item with that code), and its name,
// description and image replace the item's own when set. Items without a stored
// row follow in kit order, shown by their game code and captured icon. Stored
// rows for items the kit no longer holds are dropped. With no stored rows the
// result is the captured kit itself.
func displayKitEntries(stored []*donatev1.KitEntry, captured []KitItemView) []*donatev1.KitEntry {
	used := make([]bool, len(captured))
	out := make([]*donatev1.KitEntry, 0, len(captured))

	for _, row := range stored {
		idx := -1
		for i, c := range captured {
			if !used[i] && c.GameCode == row.GetGameCode() {
				idx = i
				break
			}
		}
		if idx < 0 {
			continue
		}
		used[idx] = true
		c := captured[idx]
		out = append(out, &donatev1.KitEntry{
			GameCode:    c.GameCode,
			Name:        firstNonEmpty(row.GetName(), c.GameCode),
			Description: row.GetDescription(),
			ImageUrl:    firstNonEmpty(row.GetImageUrl(), c.ImageURL),
			Quantity:    c.Quantity,
		})
	}

	for i, c := range captured {
		if used[i] {
			continue
		}
		out = append(out, &donatev1.KitEntry{
			GameCode: c.GameCode,
			Name:     c.GameCode,
			ImageUrl: c.ImageURL,
			Quantity: c.Quantity,
		})
	}
	return out
}

// firstNonEmpty returns the first value that is not empty.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (s *Service) BuyItem(ctx context.Context, req *donatev1.BuyItemRequest) (*donatev1.BuyItemResponse, error) {
	playerID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	purchase, err := s.purchases.Buy(ctx, playerID, req.GetItemId())
	if err != nil {
		if isDomainError(err, codes.NotFound) {
			s.opFor(ctx).Event(opPurchaseRefused).
				Entity("shop_item", req.GetItemId()).
				Reason("item not found or unavailable").
				Warn("purchase refused: the item is not on sale")
			return nil, status.Error(codes.NotFound, "item not found or unavailable")
		}
		if isDomainError(err, codes.FailedPrecondition) {
			// Insufficient funds and the closed item-type path both land here:
			// the wallet is untouched, so the record is a Warn with the reason
			// the rules gave.
			s.opFor(ctx).Event(opPurchaseRefused).
				Entity("shop_item", req.GetItemId()).
				Reason(err.Error()).
				Warn("purchase refused by the wallet or item rules")
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		s.opFor(ctx).Event(opPurchaseCompleted).
			Entity("shop_item", req.GetItemId()).
			Error("failed to buy item", err)
		return nil, status.Error(codes.Internal, "failed to buy item")
	}

	// The purchase carries the price that was actually charged and the discount
	// that produced it, so the record states the money without another read.
	// The wallet debit itself happens inside the purchase sequence, one fact
	// away: this record is the purchase, not the balance line.
	s.opFor(ctx).Event(opPurchaseCompleted).
		Entity(entityPurchase, purchase.Id).
		Field(
			zap.String("item_id", purchase.ItemID),
			zap.Int64("amount", purchase.PricePaid),
			zap.Int64("base_price", purchase.BasePrice),
			zap.Int32("discount_percent", purchase.DiscountPercent),
			zap.String("currency", coins),
			zap.String("state", string(purchase.Status)),
		).
		Info("purchase completed")

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

// kitEntriesFromProto converts the витрина rows the admin sent, keeping their
// order. It is lenient on purpose, so an older client or a round-tripped
// витрина never fails the save: a row without game_code cannot be matched to a
// captured kit item and is dropped (such rows were ignored before as well), and
// an image whose host is not allowed (e.g. a captured icon sent back as is) is
// cleared, so the row falls back to the captured icon. Quantity is not kept:
// it always comes from the kit.
func (s *Service) kitEntriesFromProto(entries []*donatev1.KitEntry) []model.KitEntry {
	result := make([]model.KitEntry, 0, len(entries))
	for _, e := range entries {
		if e.GetGameCode() == "" {
			continue
		}
		imageURL := e.GetImageUrl()
		if imageURL != "" && s.validateImageURL(imageURL) != nil {
			imageURL = ""
		}
		result = append(result, model.KitEntry{
			GameCode:    e.GetGameCode(),
			Name:        e.GetName(),
			Description: e.GetDescription(),
			ImageURL:    imageURL,
		})
	}
	return result
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
