package ierror

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

var (
	ErrNotFound       = ierror.NotFound("mail not found")
	ErrNothingToClaim = ierror.FailedPrecondition("nothing to claim")
	ErrNotClaimable   = ierror.FailedPrecondition("mail is not claimable")

	// ErrKitNotFound is returned when a kit_id was never captured by the game.
	ErrKitNotFound = ierror.NotFound("kit not found")
	// ErrKitEmpty is returned when a captured kit has no items: an empty kit is
	// treated as missing so a claimless kit-mail is never created.
	ErrKitEmpty = ierror.FailedPrecondition("kit is empty")
)
