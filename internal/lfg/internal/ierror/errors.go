package ierror

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

var (
	ErrNotFound      = ierror.NotFound("post not found")
	ErrOpenPostLimit = ierror.ResourceExhausted("open post limit reached")
	ErrPostClosed    = ierror.FailedPrecondition("post is closed or over")
	ErrOwnPost       = ierror.FailedPrecondition("cannot respond to your own post")
	ErrPostFull      = ierror.FailedPrecondition("all slots are taken")
	ErrNotAuthor     = ierror.PermissionDenied("only the author can change the post")
	ErrNotRenewable  = ierror.FailedPrecondition("only a teammate post can be renewed")
	ErrRenewTooSoon  = ierror.FailedPrecondition("the post was renewed less than a day ago")
)
