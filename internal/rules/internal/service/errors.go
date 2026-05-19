package service

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

var (
	ErrQuestionRequired = ierror.InvalidArgument("question is required")
	ErrAlreadyVerified  = ierror.AlreadyExists("user is already verified")
	ErrPermissionDenied = ierror.PermissionDenied("user does not have required permissions")
	ErrQuestionNotFound = ierror.NotFound("question not found")
)
