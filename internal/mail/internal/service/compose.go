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
// other domains (donate) call to have a mail composed. Both methods use
// purchaseID as the idempotency key, so a retry returns the same mail.
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

// ComposeItemMail composes a targeted mail granting the given items. Sender is
// "system:donate"; the mail never expires. Idempotent on purchaseID.
func (c *Composer) ComposeItemMail(ctx context.Context, recipientPlayerID, title, body, purchaseID string, items []mailcompose.ItemSpec) error {
	mail := model.NewMail(
		recipientPlayerID,
		"system:donate",
		title,
		body,
		itemSpecsToAttachments(items),
		nil,
		purchaseID,
	)
	if _, err := c.repo.CreateMail(ctx, mail); err != nil {
		c.log.Error("failed to compose item mail", zap.String("purchase_id", purchaseID), zap.Error(err))
		return err
	}
	return nil
}

// ComposeKitMail expands kitID into item attachments then composes a targeted
// mail. Sender is "system:donate"; the mail never expires. Idempotent on
// purchaseID. Fail-loud on a missing/empty kit (ErrKitNotFound / ErrKitEmpty).
func (c *Composer) ComposeKitMail(ctx context.Context, recipientPlayerID, kitID, title, body, purchaseID string) error {
	attachments, err := expandKit(ctx, c.kits, kitID)
	if err != nil {
		c.log.Error("failed to expand kit", zap.String("kit_id", kitID), zap.String("purchase_id", purchaseID), zap.Error(err))
		return err
	}
	mail := model.NewMail(
		recipientPlayerID,
		"system:donate",
		title,
		body,
		attachments,
		nil,
		purchaseID,
	)
	if _, err := c.repo.CreateMail(ctx, mail); err != nil {
		c.log.Error("failed to compose kit mail", zap.String("purchase_id", purchaseID), zap.Error(err))
		return err
	}
	return nil
}
