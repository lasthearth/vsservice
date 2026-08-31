package usecase

import "context"

// ItemSpec is one item attachment donate hands to the mail composer. Primitive
// so the mail model never crosses this seam.
type ItemSpec struct {
	GameCode     string
	Quantity     int32
	AttrSnapshot string
	Type         string
}

// MailComposer is donate's consumer-side view of the mail domain's outward
// composer port. The interface belongs to the consumer (donate); the mail
// domain's mailcompose.MailComposer is adapted to it at the composition seam in
// internal/donate/fx.go. donate may import internal/mail/mailcompose (a public
// port package); mail must never import donate.
type MailComposer interface {
	ComposeItemMail(ctx context.Context, recipientPlayerID, title, body, purchaseID string, items []ItemSpec) error
	ComposeKitMail(ctx context.Context, recipientPlayerID, kitID, title, body, purchaseID string) error
}
