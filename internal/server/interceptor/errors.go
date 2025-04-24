package interceptor

import "errors"

var (
	ErrGetUserID    = errors.New("failed to get uid from context")
	ErrGetRequestID = errors.New("failed to get rid from context")
	ErrGetClaims    = errors.New("failed to get claims from context")
)
