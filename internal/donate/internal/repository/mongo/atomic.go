package repository

import (
	"context"

	"github.com/lasthearth/vsservice/internal/donate/internal/ierror"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

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
		p, err := r.UpdatePurchase(sc, purchaseID, func(_ context.Context, p *model.Purchase) (*model.Purchase, error) {
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
		tx.AttachPurchase(purchaseID)
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
