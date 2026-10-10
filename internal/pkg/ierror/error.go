package ierror

import "google.golang.org/grpc/codes"

// DomainError is a domain-level error that carries a gRPC status code.
// Use the constructor functions to create errors; the domain interceptor
// maps them to proper gRPC status errors automatically.
type DomainError struct {
	Code    codes.Code
	Message string
}

func (e *DomainError) Error() string {
	return e.Message
}

func NotFound(msg string) *DomainError {
	return &DomainError{Code: codes.NotFound, Message: msg}
}

func InvalidArgument(msg string) *DomainError {
	return &DomainError{Code: codes.InvalidArgument, Message: msg}
}

func PermissionDenied(msg string) *DomainError {
	return &DomainError{Code: codes.PermissionDenied, Message: msg}
}

func AlreadyExists(msg string) *DomainError {
	return &DomainError{Code: codes.AlreadyExists, Message: msg}
}

func Internal(msg string) *DomainError {
	return &DomainError{Code: codes.Internal, Message: msg}
}

func Unauthenticated(msg string) *DomainError {
	return &DomainError{Code: codes.Unauthenticated, Message: msg}
}

func FailedPrecondition(msg string) *DomainError {
	return &DomainError{Code: codes.FailedPrecondition, Message: msg}
}

func ResourceExhausted(msg string) *DomainError {
	return &DomainError{Code: codes.ResourceExhausted, Message: msg}
}

// Unavailable reports a transient failure the client can retry. Use it instead of
// Internal when the cause is a dependency that was momentarily unreachable: the
// gateway answers 503, and a client knows a retry may succeed.
func Unavailable(msg string) *DomainError {
	return &DomainError{Code: codes.Unavailable, Message: msg}
}
