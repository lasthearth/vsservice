package donate

import (
	"context"

	"github.com/lasthearth/vsservice/internal/donate/internal/usecase"
	"github.com/lasthearth/vsservice/internal/mail/mailcompose"
)

// mailComposerAdapter adapts the mail domain's public mailcompose.MailComposer
// port to donate's consumer-side usecase.MailComposer. This is the composition
// seam: donate imports the public mailcompose package (allowed — mail never
// imports donate), and the two structurally-identical ItemSpec types are
// translated here so neither domain depends on the other's internals.
type mailComposerAdapter struct {
	inner mailcompose.MailComposer
}

func newMailComposerAdapter(inner mailcompose.MailComposer) usecase.MailComposer {
	return &mailComposerAdapter{inner: inner}
}

func (a *mailComposerAdapter) ComposeItemMail(ctx context.Context, recipientPlayerID, title, body, purchaseID string, items []usecase.ItemSpec) error {
	specs := make([]mailcompose.ItemSpec, len(items))
	for i, it := range items {
		specs[i] = mailcompose.ItemSpec{
			GameCode:     it.GameCode,
			Quantity:     it.Quantity,
			AttrSnapshot: it.AttrSnapshot,
			Type:         it.Type,
		}
	}
	return a.inner.ComposeItemMail(ctx, recipientPlayerID, title, body, purchaseID, specs)
}

func (a *mailComposerAdapter) ComposeKitMail(ctx context.Context, recipientPlayerID, kitID, title, body, purchaseID string) error {
	return a.inner.ComposeKitMail(ctx, recipientPlayerID, kitID, title, body, purchaseID)
}
