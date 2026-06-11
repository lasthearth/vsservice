package ierror

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

var (
	ErrNotFound            = ierror.NotFound("not found")
	ErrInsufficientFunds   = ierror.FailedPrecondition("insufficient funds")
	ErrAlreadyRefunded     = ierror.FailedPrecondition("purchase already refunded")
	ErrCannotIssueRefunded = ierror.FailedPrecondition("cannot mark refunded purchase as issued")
)
