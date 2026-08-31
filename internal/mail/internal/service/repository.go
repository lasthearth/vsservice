package service

import (
	"context"

	"github.com/lasthearth/vsservice/internal/mail/internal/model"
)

// MailRepository is the single persistence interface for the mail domain. The
// implementation lives in internal/mail/internal/repository/mongo.
type MailRepository interface {
	// Mail documents

	// CreateMail inserts a new mail. When the mail carries a non-empty
	// IdempotencyKey and a mail with that key already exists, the existing mail
	// is returned instead of inserting a duplicate.
	CreateMail(ctx context.Context, mail *model.Mail) (*model.Mail, error)

	// GetMail returns the mail by id. Returns ierror.ErrNotFound if absent.
	GetMail(ctx context.Context, id string) (*model.Mail, error)

	// UpdateMail reads the mail, applies updateFn and saves the result.
	// Returns ierror.ErrNotFound if the mail does not exist.
	UpdateMail(
		ctx context.Context,
		id string,
		updateFn func(ctx context.Context, m *model.Mail) (*model.Mail, error),
	) (*model.Mail, error)

	// ListMailsForRecipient returns mails addressed to playerID plus every
	// broadcast, newest first.
	ListMailsForRecipient(ctx context.Context, playerID string) ([]*model.Mail, error)

	// Claim rows

	// InsertClaim inserts a claim row. Returns created=false (and no error) when
	// a row for the same (mail_id, player_id) already exists — the caller then
	// falls back to UpdateClaim. This is the claimed-before-grant idempotency
	// anchor: the row is written before any physical delivery.
	InsertClaim(ctx context.Context, claim *model.MailClaim) (created bool, err error)

	// GetClaim returns the claim row for (mailID, playerID), or
	// ierror.ErrNotFound when none exists (meaning UNREAD).
	GetClaim(ctx context.Context, mailID, playerID string) (*model.MailClaim, error)

	// UpdateClaim reads the claim row for (mailID, playerID), applies updateFn
	// and saves it. Returns ierror.ErrNotFound if no row exists.
	UpdateClaim(
		ctx context.Context,
		mailID, playerID string,
		updateFn func(ctx context.Context, c *model.MailClaim) (*model.MailClaim, error),
	) (*model.MailClaim, error)

	// ListClaimsForPlayer returns every claim row belonging to playerID.
	ListClaimsForPlayer(ctx context.Context, playerID string) ([]*model.MailClaim, error)
}
