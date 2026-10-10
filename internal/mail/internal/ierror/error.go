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

	// ErrNoItems is returned when an item mail was asked for with no items. It is
	// the same failure as ErrKitEmpty one layer up: a mail whose body promises a
	// block but carries no attachment is written, and ClaimAll then skips it
	// silently, so the player loses the grant with nothing in the logs.
	ErrNoItems = ierror.FailedPrecondition("mail has no items")
	// ErrNoSender and ErrNoRecipient reject a mail that cannot be attributed or
	// cannot be delivered.
	ErrNoSender    = ierror.InvalidArgument("mail sender is empty")
	ErrNoRecipient = ierror.InvalidArgument("mail recipient is empty")
)
