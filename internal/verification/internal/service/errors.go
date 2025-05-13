package service

import (
	"errors"
)

var (
	ErrQuestionRequired     = errors.New("question is required")
	ErrAlreadyVerified      = errors.New("user is already verified")
	ErrPermissionDenied     = errors.New("user does not have required permissions")
	ErrVerificationPending  = errors.New("verification request is already under review")
)
