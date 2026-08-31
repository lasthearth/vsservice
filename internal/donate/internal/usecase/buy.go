package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/lasthearth/vsservice/internal/donate/internal/ierror"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
)

// Buy withdraws the item's effective price from the player's wallet, records the
// purchase and writes a debit ledger entry. The price and the discount that
// produced it are captured on the purchase, so a later price change does not
// rewrite history.
//
// Write order: wallet withdrawal, then purchase record, then ledger entry.
// Nothing here is atomic (see Sequence) — what a mid-sequence failure leaves:
//
//   - withdrawal fails: nothing written; the caller is told why (insufficient
//     funds, no wallet).
//   - purchase record fails: the coins are already gone. Compensated here by
//     crediting them back, which is a single $inc upsert and safe to run
//     because no purchase exists to double-refund. If that credit ALSO fails
//     the coins are lost and the returned error names both failures — no
//     self-healing, it needs a human.
//   - ledger entry fails: the wallet is debited and the purchase exists, so
//     the player has what they paid for and the only casualty is a missing
//     ledger row. Not compensated: the purchase is the record of truth and the
//     ledger is a report. It does not self-heal; ListTransactions will be short
//     one debit for this player. The call still reports failure, as it did
//     before this rule moved out of the repository.
func (uc *Purchases) Buy(ctx context.Context, playerID, itemID string) (*model.Purchase, error) {
	item, err := uc.repo.GetShopItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if !item.IsAvailable {
		return nil, ierror.ErrNotFound
	}

	// Resolve the player name from the wallet (set by admin via AddCoins);
	// fall back to empty when the player has no wallet yet — the withdrawal
	// below will then fail anyway.
	playerName := ""
	if wallet, werr := uc.repo.GetWalletByPlayerID(ctx, playerID); werr == nil {
		playerName = wallet.PlayerName
	}

	now := time.Now()
	price := item.EffectivePriceAt(now)
	discountPercent := int32(0)
	if item.DiscountActive(now) {
		discountPercent = item.DiscountPercent
	}

	var purchase *model.Purchase
	err = uc.seq.Do(ctx,
		func(ctx context.Context) error {
			return uc.repo.UpdateWallet(ctx, playerID, func(_ context.Context, w *model.Wallet) (*model.Wallet, error) {
				if err := w.Withdraw(price); err != nil {
					return nil, ierror.ErrInsufficientFunds
				}
				return w, nil
			})
		},
		func(ctx context.Context) error {
			p, err := uc.repo.CreatePurchase(ctx, model.NewPurchase(
				playerID, playerName, item.Id, item.Name, price, item.Price, discountPercent,
			))
			if err != nil {
				if _, cerr := uc.repo.AddCoinsToWallet(ctx, playerID, playerName, price); cerr != nil {
					return fmt.Errorf("purchase record failed (%w) and returning the %d withdrawn coins failed: %w", err, price, cerr)
				}
				return err
			}
			purchase = p
			return nil
		},
		func(ctx context.Context) error {
			tx := model.NewDebitTransaction(playerID, price, "purchase: "+item.Name)
			tx.AttachPurchase(purchase.Id)
			_, err := uc.repo.CreateTransaction(ctx, tx)
			return err
		},
		// Compose the delivery mail and stamp the purchase issued. This is the
		// donate→mail seam: an item purchase becomes an item-mail, a kit purchase
		// a kit-mail (expanded server-side from the kit code == item.Code). Both
		// composes are idempotent on purchase.Id, so a retry after a mid-sequence
		// failure returns the same mail. If compose fails the seq returns the
		// error: the purchase exists but is not stamped issued (reconcile is fog
		// — not built here). Mirrors the ledger step's "purchase is the record of
		// truth" semantics.
		func(ctx context.Context) error {
			title := "Покупка: " + item.Name
			body := fmt.Sprintf("Оплачено %d монет (базовая цена %d, скидка %d%%).", price, item.Price, discountPercent)

			switch item.Type {
			case model.ItemTypeKit:
				if err := uc.mail.ComposeKitMail(ctx, playerID, item.Code, title, body, purchase.Id); err != nil {
					return err
				}
			default: // model.ItemTypeItem
				if err := uc.mail.ComposeItemMail(ctx, playerID, title, body, purchase.Id, []ItemSpec{
					{GameCode: item.Code, Quantity: 1, AttrSnapshot: "", Type: "item"},
				}); err != nil {
					return err
				}
			}

			updated, err := uc.repo.UpdatePurchase(ctx, purchase.Id, func(_ context.Context, p *model.Purchase) (*model.Purchase, error) {
				if err := p.MarkIssuedBy("system:mail"); err != nil {
					return nil, ierror.ErrCannotIssueRefunded
				}
				return p, nil
			})
			if err != nil {
				return err
			}
			purchase = updated
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return purchase, nil
}
