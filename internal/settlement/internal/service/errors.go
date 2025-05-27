package service

import (
	"errors"
)

var (
	ErrSettlementNotFound = errors.New("settlement not found")
	ErrPermissionDenied   = errors.New("user does not have required permissions")
	ErrAlreadyApproved    = errors.New("settlement is already approved")
)
