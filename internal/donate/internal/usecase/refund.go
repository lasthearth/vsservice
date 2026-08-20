package usecase

import (
	"context"

	"github.com/lasthearth/vsservice/internal/donate/internal/ierror"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
)

// Refund marks the purchase refunded, returns the price paid to the wallet and
// writes a credit ledger entry.
//
// Write order: purchase status first, then the wallet credit, then the ledger
// entry. Marking first is what makes a retry safe — a second Refund of the same
// purchase is rejected with ErrAlreadyRefunded instead of paying twice. What a
// mid-sequence failure leaves:
//
//   - marking fails: nothing written (not found, already refunded).
//   - wallet credit fails: the purchase reads "refunded" but the player was not
//     paid. NOT compensated: un-marking would need an un-refund on the model
//     that no legitimate caller should have, and reversing the order would
//     risk paying twice on retry, which is worse. It does not self-heal, and a
//     retry will now return ErrAlreadyRefunded — the state is visible (status
//     refunded with no matching credit in the ledger) and needs an admin to
//     re-credit via AddCoins.
//   - ledger entry fails: the player has their coins and the purchase is
//     refunded; only the ledger row is missing. Not compensated, does not
//     self-heal, same reasoning as Buy.
func (uc *Purchases) Refund(ctx context.Context, purchaseID, reason string) (*model.Purchase, error) {
	var purchase *model.Purchase
	err := uc.seq.Do(ctx,
		func(ctx context.Context) error {
			p, err := uc.repo.UpdatePurchase(ctx, purchaseID, func(_ context.Context, p *model.Purchase) (*model.Purchase, error) {
				if err := p.Refund(); err != nil {
					return nil, ierror.ErrAlreadyRefunded
				}
				return p, nil
			})
			if err != nil {
				return err
			}
			purchase = p
			return nil
		},
		func(ctx context.Context) error {
			_, err := uc.repo.AddCoinsToWallet(ctx, purchase.PlayerID, purchase.PlayerName, purchase.PricePaid)
			return err
		},
		func(ctx context.Context) error {
			tx := model.NewCreditTransaction(purchase.PlayerID, purchase.PricePaid, reason)
			tx.AttachPurchase(purchaseID)
			_, err := uc.repo.CreateTransaction(ctx, tx)
			return err
		},
	)
	if err != nil {
		return nil, err
	}

	return purchase, nil
}
