package ierror

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

var (
	ErrNotFound           = ierror.NotFound("ticket not found")
	ErrReplyNotFound      = ierror.NotFound("saved reply not found")
	ErrBadPageToken       = ierror.InvalidArgument("malformed page_token")
	ErrActiveTicketLimit  = ierror.ResourceExhausted("active ticket limit reached")
	ErrSavedReplyLimit    = ierror.ResourceExhausted("saved reply limit reached")
	ErrClosed             = ierror.FailedPrecondition("the ticket is closed")
	ErrNotClosed          = ierror.FailedPrecondition("the ticket is not closed")
	ErrThreadFull         = ierror.FailedPrecondition("the ticket holds too many messages")
	ErrCategoryInvalid    = ierror.InvalidArgument("unknown category")
	ErrStatusInvalid      = ierror.InvalidArgument("unknown status")
	ErrSubjectInvalid     = ierror.InvalidArgument("subject must be 3 to 120 characters")
	ErrTextInvalid        = ierror.InvalidArgument("text must be 1 to 4000 characters")
	ErrReplyTitleInvalid  = ierror.InvalidArgument("title must be 1 to 80 characters")
	ErrCloseWithNote      = ierror.InvalidArgument("an internal note cannot close the ticket")
	ErrAuthorRequired     = ierror.InvalidArgument("author is required")
	ErrPermissionRequired = ierror.PermissionDenied("tickets:manage scope required")
)
