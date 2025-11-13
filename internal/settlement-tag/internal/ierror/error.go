package ierror

import "errors"

var (
	ErrNotFound                = errors.New("tag not found")
	ErrAlreadyExists           = errors.New("tag already exists")
	ErrInvalidArgument         = errors.New("invalid argument")
	ErrValidationError         = errors.New("validation error")
	ErrConstraintViolation     = errors.New("constraint violation")
	ErrSettlementNotFound      = errors.New("settlement not found")
	ErrTagNotActive = errors.New("tag is not active")
)           
