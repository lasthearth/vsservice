package model

import (
	"time"
)

// MailState is the per-player state of a mail. UNREAD is the implicit state
// when no claim row exists; EXPIRED and REVOKED are derived at read from the
// mail document, never stored on the row.
type MailState string

const (
	MailStateUnread  MailState = "unread"
	MailStateRead    MailState = "read"
	MailStateClaimed MailState = "claimed"
	MailStateExpired MailState = "expired"
	MailStateRevoked MailState = "revoked"
)

// MailClaim is the per-player state row for one (MailID, PlayerID) pair. It is
// created lazily: the absence of a row means UNREAD.
type MailClaim struct {
	Id        string
	MailID    string
	PlayerID  string
	State     MailState
	ReadAt    *time.Time
	ClaimedAt *time.Time
}

// NewMailClaim creates a fresh UNREAD claim row for a (mailID, playerID) pair.
func NewMailClaim(mailID, playerID string) *MailClaim {
	return &MailClaim{
		MailID:   mailID,
		PlayerID: playerID,
		State:    MailStateUnread,
	}
}

// ReconstituteMailClaim rebuilds a MailClaim from persisted state. Repository use only.
func ReconstituteMailClaim(id, mailID, playerID string, state MailState, readAt, claimedAt *time.Time) *MailClaim {
	return &MailClaim{
		Id:        id,
		MailID:    mailID,
		PlayerID:  playerID,
		State:     state,
		ReadAt:    readAt,
		ClaimedAt: claimedAt,
	}
}

// MarkCreated records the persisted identity.
func (c *MailClaim) MarkCreated(id string) { c.Id = id }

// IsClaimed reports whether the attachments were already claimed. A claimed row
// is terminal and never rolled back.
func (c *MailClaim) IsClaimed() bool { return c.State == MailStateClaimed }

// MarkRead transitions unread → read. Idempotent once read or beyond; a claimed
// row stays claimed. Returns ErrMailClaimTerminal only when the row is in a
// terminal non-read state that read cannot follow.
func (c *MailClaim) MarkRead() error {
	switch c.State {
	case MailStateUnread:
		now := time.Now()
		c.State = MailStateRead
		c.ReadAt = &now
		return nil
	case MailStateRead, MailStateClaimed:
		// Already read (or beyond): idempotent no-op.
		return nil
	default:
		// expired / revoked are derived, not stored, so a stored row should
		// never be here; guard anyway.
		return ErrMailClaimTerminal
	}
}

// Claim transitions read/unread → claimed. Idempotent when already claimed.
// hasAttachments guards the notification case (nothing to claim). Returns
// ErrNothingToClaim when the mail carries no attachments.
func (c *MailClaim) Claim(hasAttachments bool) error {
	if c.State == MailStateClaimed {
		return nil
	}
	if !hasAttachments {
		return ErrNothingToClaim
	}
	now := time.Now()
	if c.ReadAt == nil {
		c.ReadAt = &now
	}
	c.State = MailStateClaimed
	c.ClaimedAt = &now
	return nil
}
