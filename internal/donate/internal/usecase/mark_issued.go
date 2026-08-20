package usecase

import (
	"context"

	"github.com/lasthearth/vsservice/internal/donate/internal/ierror"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
)

// MarkIssued records that adminID manually delivered the purchase. Single
// write, so there is no sequence and no partial failure: idempotent on an
// already-issued purchase, ErrCannotIssueRefunded on a refunded one,
// ErrNotFound when the purchase is missing.
func (uc *Purchases) MarkIssued(ctx context.Context, purchaseID, adminID string) (*model.Purchase, error) {
	return uc.repo.UpdatePurchase(ctx, purchaseID, func(_ context.Context, p *model.Purchase) (*model.Purchase, error) {
		if err := p.MarkIssued(adminID); err != nil {
			return nil, ierror.ErrCannotIssueRefunded
		}
		return p, nil
	})
}
