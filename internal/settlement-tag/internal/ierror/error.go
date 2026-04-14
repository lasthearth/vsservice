package ierror

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

var (
	ErrNotFound            = ierror.NotFound("tag not found")
	ErrAlreadyExists       = ierror.AlreadyExists("tag already exists")
	ErrInvalidArgument     = ierror.InvalidArgument("invalid argument")
	ErrValidationError     = ierror.InvalidArgument("validation error")
	ErrConstraintViolation = ierror.FailedPrecondition("constraint violation")
	ErrSettlementNotFound  = ierror.NotFound("settlement not found")
	ErrTagNotActive        = ierror.FailedPrecondition("tag is not active")
)
