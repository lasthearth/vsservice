package model

import (
	"regexp"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/ierror"
)

// NicknameCooldown is the wait between two nickname changes a player makes
// themselves. A moderator grant clears it; a change - self-service or made by
// an admin - starts it again from the moment of the change.
const NicknameCooldown = 183 * 24 * time.Hour

// Nickname errors are domain errors: the interceptor answers the gRPC code
// they carry, so the service never spells one out.
var (
	ErrNicknameInvalid   = ierror.InvalidArgument("invalid nickname format")
	ErrNicknameUnchanged = ierror.InvalidArgument("new nickname must be different from the current one")
	// ErrNicknameAlreadyChangeable refuses a grant that would give nothing: the
	// player has no recorded change, so the cooldown already lets them through.
	ErrNicknameAlreadyChangeable = ierror.FailedPrecondition(
		"the player can already change the nickname",
	)
)

// nicknamePattern is the shape the game accepts: a name the server can put in
// its tab list. Compiling it once keeps a validator off the request path, which
// matters because it runs for every change, self-service or admin.
var nicknamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,15}$`)

// ValidNickname reports whether the game accepts this nickname.
func ValidNickname(nickname string) bool {
	return nicknamePattern.MatchString(nickname)
}

// NicknameChangeAllowedAt returns the earliest moment the player may change
// their nickname themselves. The zero time means no change is recorded, so
// there is nothing to wait for.
func (p *Player) NicknameChangeAllowedAt() time.Time {
	if p.LastNicknameChangedAt.IsZero() {
		return time.Time{}
	}

	return p.LastNicknameChangedAt.Add(NicknameCooldown)
}

// CanChangeNickname reports whether the player may change the nickname at now.
func (p *Player) CanChangeNickname(now time.Time) bool {
	allowedAt := p.NicknameChangeAllowedAt()
	return allowedAt.IsZero() || !now.Before(allowedAt)
}

// CheckNicknameChange reports why nickname cannot replace the current one, if
// it cannot. It leaves the player unchanged, so a caller can check before it
// takes the time to read or write anything else.
func (p *Player) CheckNicknameChange(nickname string) error {
	if !ValidNickname(nickname) {
		return ErrNicknameInvalid
	}

	if p.UserGameName == nickname {
		return ErrNicknameUnchanged
	}

	return nil
}

// SetNickname replaces the nickname, keeps the old one and records the moment
// of the change, which starts the cooldown. Both the self-service path and the
// admin path go through it, so the rule lives in one place.
func (p *Player) SetNickname(nickname string, now time.Time) error {
	if err := p.CheckNicknameChange(nickname); err != nil {
		return err
	}

	p.PreviousNickname = p.UserGameName
	p.UserGameName = nickname
	p.LastNicknameChangedAt = now

	return nil
}

// GrantNicknameChange clears the recorded change, so the cooldown no longer
// holds and the player may change the nickname at once. A player with no
// recorded change gains nothing from a grant, so that request is refused
// instead of notifying them about a permission they already had.
func (p *Player) GrantNicknameChange() error {
	if p.LastNicknameChangedAt.IsZero() {
		return ErrNicknameAlreadyChangeable
	}

	p.LastNicknameChangedAt = time.Time{}

	return nil
}
