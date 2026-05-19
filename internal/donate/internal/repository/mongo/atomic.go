package repository

import (
	"context"

	"github.com/lasthearth/vsservice/internal/donate/internal/ierror"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

// BuyItem atomically deducts coins, creates a purchase, and records a debit
// transaction — all within a single MongoDB session.
func (r *Repository) BuyItem(ctx context.Context, playerID, itemID string) (*model.Purchase, error) {
	l := r.log.With(
		zap.String("method", "BuyItem"),
		zap.String("player_id", playerID),
		zap.String("item_id", itemID),
	)

	item, err := r.GetShopItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if !item.IsAvailable {
		return nil, ierror.ErrNotFound
	}

	// Resolve player name from wallet (set by admin via AddCoins); fall back to empty.
	playerName := ""
	if wallet, err := r.GetWalletByPlayerID(ctx, playerID); err == nil {
		playerName = wallet.PlayerName
	}

	session, err := r.client.StartSession()
	if err != nil {
		l.Error("failed to start session", zap.Error(err))
		return nil, err
	}
	defer session.EndSession(ctx)

	var purchase *model.Purchase

	err = mongo.WithSession(ctx, session, func(sc context.Context) error {
		if err := r.UpdateWallet(sc, playerID, func(sc context.Context, w *model.Wallet) (*model.Wallet, error) {
			if err := w.Withdraw(item.Price); err != nil {
				return nil, ierror.ErrInsufficientFunds
			}
			return w, nil
		}); err != nil {
			return err
		}

		p, err := r.createPurchase(sc, model.NewPurchase(playerID, playerName, item.Id, item.Name, item.Price))
		if err != nil {
			l.Error("failed to create purchase record", zap.Error(err))
			return err
		}
		purchase = p

		tx := model.NewDebitTransaction(playerID, item.Price, "purchase: "+item.Name)
		tx.PurchaseID = purchase.Id
		if _, err := r.CreateTransaction(sc, tx); err != nil {
			l.Error("failed to record transaction", zap.Error(err))
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	l.Info("item purchased", zap.String("purchase_id", purchase.Id))
	return purchase, nil
}

// Refund atomically marks the purchase as refunded, restores coins to the wallet,
// and records a credit transaction — all within a single MongoDB session.
func (r *Repository) Refund(ctx context.Context, purchaseID, reason string) (*model.Purchase, error) {
	l := r.log.With(zap.String("method", "Refund"), zap.String("purchase_id", purchaseID))

	session, err := r.client.StartSession()
	if err != nil {
		l.Error("failed to start session", zap.Error(err))
		return nil, err
	}
	defer session.EndSession(ctx)

	var purchase *model.Purchase

	err = mongo.WithSession(ctx, session, func(sc context.Context) error {
		p, err := r.updatePurchase(sc, purchaseID, func(_ context.Context, p *model.Purchase) (*model.Purchase, error) {
			if err := p.Refund(); err != nil {
				return nil, ierror.ErrAlreadyRefunded
			}
			return p, nil
		})
		if err != nil {
			return err
		}
		purchase = p

		if _, err := r.AddCoinsToWallet(sc, p.PlayerID, p.PlayerName, p.PricePaid); err != nil {
			l.Error("failed to restore coins", zap.Error(err))
			return err
		}

		tx := model.NewCreditTransaction(p.PlayerID, p.PricePaid, reason)
		tx.PurchaseID = purchaseID
		if _, err := r.CreateTransaction(sc, tx); err != nil {
			l.Error("failed to record refund transaction", zap.Error(err))
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	l.Info("purchase refunded", zap.String("purchase_id", purchaseID))
	return purchase, nil
}
