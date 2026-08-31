package model

import "time"

// Mail is the immutable content document. After creation only the revoked flag
// mutates; per-player state lives in MailClaim, not here.
type Mail struct {
	Id             string
	Recipient      string // a user_id or the literal RecipientBroadcast
	Sender         string // e.g. "admin:{jwt_sub}" or "system:donate"
	Title          string
	Body           string
	Attachments    []Attachment
	CreatedAt      time.Time
	ExpiresAt      *time.Time // nil = never expires
	Revoked        bool
	IdempotencyKey string // empty = no dedup key
}

// RecipientBroadcast is the sentinel recipient for a mail addressed to everyone.
const RecipientBroadcast = "broadcast"

// NewMail creates a content document addressed to a single recipient (a user_id
// or RecipientBroadcast). expiresAt nil means the mail never expires.
func NewMail(recipient, sender, title, body string, attachments []Attachment, expiresAt *time.Time, idempotencyKey string) *Mail {
	return &Mail{
		Recipient:      recipient,
		Sender:         sender,
		Title:          title,
		Body:           body,
		Attachments:    attachments,
		ExpiresAt:      expiresAt,
		IdempotencyKey: idempotencyKey,
	}
}

// ReconstituteMail rebuilds a Mail from persisted state. Repository use only.
func ReconstituteMail(
	id, recipient, sender, title, body string,
	attachments []Attachment,
	createdAt time.Time,
	expiresAt *time.Time,
	revoked bool,
	idempotencyKey string,
) *Mail {
	return &Mail{
		Id:             id,
		Recipient:      recipient,
		Sender:         sender,
		Title:          title,
		Body:           body,
		Attachments:    attachments,
		CreatedAt:      createdAt,
		ExpiresAt:      expiresAt,
		Revoked:        revoked,
		IdempotencyKey: idempotencyKey,
	}
}

// MarkCreated records the persisted identity and creation time.
func (m *Mail) MarkCreated(id string, createdAt time.Time) {
	m.Id = id
	m.CreatedAt = createdAt
}

// Revoke sets the revoked flag. Idempotent: re-revoking is a no-op.
func (m *Mail) Revoke() {
	m.Revoked = true
}

// HasAttachments reports whether the mail carries anything to claim.
func (m *Mail) HasAttachments() bool {
	return len(m.Attachments) > 0
}

// IsExpiredAt reports whether the mail has passed its expiry at t.
func (m *Mail) IsExpiredAt(t time.Time) bool {
	return m.ExpiresAt != nil && !t.Before(*m.ExpiresAt)
}
