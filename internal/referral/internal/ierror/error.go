package ierror

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

var (
	ErrNotFound        = ierror.NotFound("not found")
	ErrAlreadyReferred = ierror.FailedPrecondition("referral code already used")
)
