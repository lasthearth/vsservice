// Package mailcompose is the mail domain's PUBLIC outward port: the primitive-
// typed surface other domains (e.g. donate) call to have a mail composed on
// their behalf. It carries no mail model or DTO types so callers never import
// mail internals, and mail never imports the caller — the seam stays acyclic.
package mailcompose

import "context"

// ItemSpec is one item to attach to a composed mail. It mirrors mail's
// ItemAttachment in primitive form: GameCode + Type + Quantity identify the
// asset, AttrSnapshot is an opaque base64 TreeAttribute snapshot ("" = plain
// stack). Validity is the game's concern (checked at claim), not here.
type ItemSpec struct {
	GameCode     string
	Quantity     int32
	AttrSnapshot string
	Type         string
}

// Sender tags for system mails. The tag is shown to the player as the mail's
// sender.
const (
	// SenderSettlement marks mails the settlement domain sends on its own
	// (notifier delivery, level-up notices).
	SenderSettlement = "system:settlement"
)

// MailComposer composes a mail addressed to a single player. Implemented inside
// the mail domain and bound via fx.As in internal/mail/fx.go. Every method is
// idempotent on its key: the same key returns the same mail. When the context
// carries a Mongo session, the mail is written inside that session's
// transaction, so a caller can make the mail atomic with its own writes.
type MailComposer interface {
	// ComposeItemMail creates a mail granting the given items to recipientPlayerID.
	ComposeItemMail(ctx context.Context, recipientPlayerID, title, body, purchaseID string, items []ItemSpec) error
	// ComposeKitMail creates a mail whose attachments are kitID's contents,
	// expanded server-side. NotFound if the kit was never captured;
	// FailedPrecondition if it is empty.
	ComposeKitMail(ctx context.Context, recipientPlayerID, kitID, title, body, purchaseID string) error
	// ComposeSystemItemMail is ComposeItemMail with a caller-chosen sender tag
	// (see the Sender* constants), for mails that are not donate purchases.
	ComposeSystemItemMail(ctx context.Context, sender, recipientPlayerID, title, body, idempotencyKey string, items []ItemSpec) error
	// ComposeNotificationMail creates a plain mail without attachments: a
	// notice the player only reads. The mail never expires.
	ComposeNotificationMail(ctx context.Context, sender, recipientPlayerID, title, body, idempotencyKey string) error
}
