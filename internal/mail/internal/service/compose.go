package service

import (
	"context"
	"errors"

	mailerr "github.com/lasthearth/vsservice/internal/mail/internal/ierror"
	"github.com/lasthearth/vsservice/internal/mail/internal/model"
	"github.com/lasthearth/vsservice/internal/mail/mailcompose"
	pkgerr "github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
)

// KitReader is the mail-side consumer port for reading a captured kit's
// contents. kitdef's read service satisfies it through an adapter at the
// composition seam (internal/mail/fx.go), so mail never imports kitdef
// internals and kitdef never imports mail.
type KitReader interface {
	GetKit(ctx context.Context, code string) (*KitSnapshot, error)
}

// KitSnapshot is a captured kit's contents in mail's own vocabulary.
type KitSnapshot struct {
	Items []KitItem
}

// KitItem is one kit entry. Fields are carried through to the mail attachment
// as-is — mail never parses AttrSnapshot (image_url is deliberately not
// carried; temperature/transitionstate stripping is an at-claim concern).
type KitItem struct {
	GameCode     string
	Type         string
	AttrSnapshot string
	Quantity     int32
}

// expandKit reads kitID via the KitReader and maps its items to mail item
// attachments. Fail-loud: a kit that was never captured is ErrKitNotFound; a
// captured-but-empty kit is ErrKitEmpty (empty ≡ missing — never a claimless
// kit-mail).
func expandKit(ctx context.Context, kits KitReader, kitID string) ([]model.Attachment, error) {
	snap, err := kits.GetKit(ctx, kitID)
	if err != nil {
		// kitdef signals an uncaptured kit with a NotFound DomainError. Normalize
		// any NotFound to mail's own ErrKitNotFound; anything else is internal.
		var de *pkgerr.DomainError
		if errors.As(err, &de) && de.Code == codes.NotFound {
			return nil, mailerr.ErrKitNotFound
		}
		return nil, err
	}
	if len(snap.Items) == 0 {
		return nil, mailerr.ErrKitEmpty
	}
	out := make([]model.Attachment, len(snap.Items))
	for i, it := range snap.Items {
		out[i] = model.Attachment{Item: &model.ItemAttachment{
			GameCode:     it.GameCode,
			Quantity:     it.Quantity,
			AttrSnapshot: it.AttrSnapshot,
			Type:         it.Type,
		}}
	}
	return out, nil
}

// itemSpecsToAttachments maps donate's primitive ItemSpecs to mail attachments.
func itemSpecsToAttachments(items []mailcompose.ItemSpec) []model.Attachment {
	if items == nil {
		return nil
	}
	out := make([]model.Attachment, len(items))
	for i, it := range items {
		out[i] = model.Attachment{Item: &model.ItemAttachment{
			GameCode:     it.GameCode,
			Quantity:     it.Quantity,
			AttrSnapshot: it.AttrSnapshot,
			Type:         it.Type,
		}}
	}
	return out
}

// Composer implements mailcompose.MailComposer — the mail domain's outward port
// other domains (donate) call to have a mail composed. Every method uses the
// caller's idempotency key, so a retry returns the same mail.
type Composer struct {
	repo MailRepository
	kits KitReader
	log  logger.Logger
}

var _ mailcompose.MailComposer = (*Composer)(nil)

// ComposerOpts wires the composer from the same repository and kit reader the
// service uses.
type ComposerOpts struct {
	fx.In

	Repo   MailRepository
	Kits   KitReader
	Logger logger.Logger
}

func NewComposer(opts ComposerOpts) *Composer {
	return &Composer{repo: opts.Repo, kits: opts.Kits, log: opts.Logger}
}

// validateEnvelope rejects a mail that cannot be attributed or cannot be
// delivered. composeMail calls it, so no exported path can persist an
// addressless mail.
func (c *Composer) validateEnvelope(sender, recipient, key string) error {
	var err error
	switch {
	case sender == "":
		err = mailerr.ErrNoSender
	case recipient == "":
		err = mailerr.ErrNoRecipient
	}
	if err != nil {
		c.log.Error("refusing to compose mail",
			zap.String("idempotency_key", key),
			zap.String("sender", sender),
			zap.String("recipient", recipient),
			zap.Error(err))
	}
	return err
}

// validateItems rejects an item mail with nothing to grant. Without it a caller
// mistake persists a mail whose body promises a block and whose attachments are
// empty, and ClaimAll skips that mail silently: the player loses the grant and
// nothing in the logs says so.
func (c *Composer) validateItems(items []mailcompose.ItemSpec, key string) error {
	if len(items) > 0 {
		return nil
	}
	c.log.Error("refusing to compose an item mail with no items",
		zap.String("idempotency_key", key), zap.Error(mailerr.ErrNoItems))
	return mailerr.ErrNoItems
}

// composeMail is the one primitive every exported method funnels through:
// validate the envelope, build the mail, write it, and log a failure with the
// key that identifies it. Nil attachments mean a notice with nothing to claim.
// The mail never expires; expiry is the admin ComposeMail rpc's business, not
// this port's.
func (c *Composer) composeMail(ctx context.Context, sender, recipient, title, body, key string, attachments []model.Attachment) error {
	if err := c.validateEnvelope(sender, recipient, key); err != nil {
		return err
	}

	mail := model.NewMail(recipient, sender, title, body, attachments, nil, key)
	if _, err := c.repo.CreateMail(ctx, mail); err != nil {
		c.log.Error("failed to compose mail",
			zap.String("idempotency_key", key),
			zap.String("sender", sender),
			zap.String("recipient", recipient),
			zap.Error(err))
		return err
	}
	return nil
}

// composeItemMail is composeMail for a grant: it refuses to write a mail that
// promises items and carries none.
func (c *Composer) composeItemMail(ctx context.Context, sender, recipient, title, body, key string, items []mailcompose.ItemSpec) error {
	if err := c.validateItems(items, key); err != nil {
		return err
	}
	return c.composeMail(ctx, sender, recipient, title, body, key, itemSpecsToAttachments(items))
}

// ComposeItemMail composes a targeted mail granting the given items. Sender is
// SenderDonate; the mail never expires. Idempotent on purchaseID.
func (c *Composer) ComposeItemMail(ctx context.Context, recipientPlayerID, title, body, purchaseID string, items []mailcompose.ItemSpec) error {
	return c.composeItemMail(ctx, mailcompose.SenderDonate, recipientPlayerID, title, body, purchaseID, items)
}

// ComposeSystemItemMail composes a targeted mail granting the given items on
// behalf of a system sender. Idempotent on idempotencyKey.
func (c *Composer) ComposeSystemItemMail(ctx context.Context, sender, recipientPlayerID, title, body, idempotencyKey string, items []mailcompose.ItemSpec) error {
	return c.composeItemMail(ctx, sender, recipientPlayerID, title, body, idempotencyKey, items)
}

// ComposeNotificationMail composes a targeted mail with no attachments — a
// notice the player only reads. The mail never expires. Idempotent on
// idempotencyKey.
func (c *Composer) ComposeNotificationMail(ctx context.Context, sender, recipientPlayerID, title, body, idempotencyKey string) error {
	return c.composeMail(ctx, sender, recipientPlayerID, title, body, idempotencyKey, nil)
}

// ComposeKitMail expands kitID into item attachments then composes a targeted
// mail. Sender is SenderDonate; the mail never expires. Idempotent on
// purchaseID. Fail-loud on a missing/empty kit (ErrKitNotFound / ErrKitEmpty).
func (c *Composer) ComposeKitMail(ctx context.Context, recipientPlayerID, kitID, title, body, purchaseID string) error {
	// Checked before the kit read, so a bad recipient costs no lookup. composeMail
	// checks it again; the second check is two string comparisons.
	if err := c.validateEnvelope(mailcompose.SenderDonate, recipientPlayerID, purchaseID); err != nil {
		return err
	}

	attachments, err := expandKit(ctx, c.kits, kitID)
	if err != nil {
		c.log.Error("failed to expand kit", zap.String("kit_id", kitID), zap.String("purchase_id", purchaseID), zap.Error(err))
		return err
	}
	return c.composeMail(ctx, mailcompose.SenderDonate, recipientPlayerID, title, body, purchaseID, attachments)
}
