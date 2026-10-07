package model

import (
	"crypto/rand"
	"errors"
	"time"
)

const (
	// InviteLinkMaxTTL is the longest an invite link may stay valid.
	InviteLinkMaxTTL = 30 * 24 * time.Hour
	// InviteLinkMaxUses caps how many players one link may let in.
	InviteLinkMaxUses = 100
	// MaxActiveInviteLinks caps the usable links a settlement keeps at once.
	MaxActiveInviteLinks = 10

	// inviteCodeLen is short enough to read aloud and long enough not to guess:
	// 32^10 ≈ 10^15 codes.
	inviteCodeLen = 10
	// inviteCodeAlphabet is Crockford base32: no I, L, O, U to misread.
	inviteCodeAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
)

var (
	ErrInviteTTLInvalid     = errors.New("invite link ttl must be between 0 and 30 days")
	ErrInviteMaxUsesInvalid = errors.New("invite link max uses must be between 0 and 100")
	ErrInviteExpired        = errors.New("invite link has expired")
	ErrInviteExhausted      = errors.New("invite link has no uses left")
	ErrInviteRevoked        = errors.New("invite link was revoked")
)

// InviteLinkStatus is whether a link can still be used, and if not, why.
type InviteLinkStatus string

const (
	InviteLinkActive    InviteLinkStatus = "active"
	InviteLinkExpired   InviteLinkStatus = "expired"
	InviteLinkExhausted InviteLinkStatus = "exhausted"
	InviteLinkRevoked   InviteLinkStatus = "revoked"
)

// InviteLink is a shareable code that lets players join a settlement without a
// join request: whoever created it holds the invite permission, so using it is
// an invitation accepted in advance.
type InviteLink struct {
	Id           string
	SettlementId string
	Code         string
	CreatedBy    string
	// MaxUses is 0 for unlimited.
	MaxUses int32
	Uses    int32
	// ExpiresAt is nil for a link that never expires.
	ExpiresAt *time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewInviteLink builds a link with a fresh random code. ttl 0 means the link
// does not expire; maxUses 0 means it is unlimited.
func NewInviteLink(settlementId, createdBy string, ttl time.Duration, maxUses int32, now time.Time) (*InviteLink, error) {
	if ttl < 0 || ttl > InviteLinkMaxTTL {
		return nil, ErrInviteTTLInvalid
	}
	if maxUses < 0 || maxUses > InviteLinkMaxUses {
		return nil, ErrInviteMaxUsesInvalid
	}

	code, err := newInviteCode()
	if err != nil {
		return nil, err
	}

	link := &InviteLink{
		SettlementId: settlementId,
		Code:         code,
		CreatedBy:    createdBy,
		MaxUses:      maxUses,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if ttl > 0 {
		expires := now.Add(ttl)
		link.ExpiresAt = &expires
	}

	return link, nil
}

// Status reports whether the link is usable at now. Revocation wins over
// expiry, expiry over exhaustion.
func (l *InviteLink) Status(now time.Time) InviteLinkStatus {
	switch {
	case l.RevokedAt != nil:
		return InviteLinkRevoked
	case l.ExpiresAt != nil && !now.Before(*l.ExpiresAt):
		return InviteLinkExpired
	case l.MaxUses > 0 && l.Uses >= l.MaxUses:
		return InviteLinkExhausted
	default:
		return InviteLinkActive
	}
}

// Use spends one use of the link, or explains why it cannot be used.
func (l *InviteLink) Use(now time.Time) error {
	switch l.Status(now) {
	case InviteLinkRevoked:
		return ErrInviteRevoked
	case InviteLinkExpired:
		return ErrInviteExpired
	case InviteLinkExhausted:
		return ErrInviteExhausted
	}

	l.Uses++
	return nil
}

// Revoke stops the link from working. Revoking twice keeps the first moment.
func (l *InviteLink) Revoke(now time.Time) {
	if l.RevokedAt == nil {
		l.RevokedAt = &now
	}
}

// Touch stamps the persisted update time (called by mongox.UpdateDoc).
func (l *InviteLink) Touch(now time.Time) { l.UpdatedAt = now }

// newInviteCode returns inviteCodeLen characters of inviteCodeAlphabet from
// crypto/rand. 256 is a multiple of 32, so taking the low five bits is unbiased.
func newInviteCode() (string, error) {
	buf := make([]byte, inviteCodeLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i, b := range buf {
		buf[i] = inviteCodeAlphabet[b&31]
	}
	return string(buf), nil
}
