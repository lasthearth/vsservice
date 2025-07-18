package ssorepository

import "errors"

var (
	ErrHTTPRequestFailed = errors.New("failed to make HTTP request")
	ErrHTTPStatusNotOK   = errors.New("HTTP status not OK")
	ErrReadResponseBody  = errors.New("failed to read response body")
	ErrUnmarshalJSON     = errors.New("failed to unmarshal JSON")
	ErrFailedCreateReq   = errors.New("failed to create request")
	ErrMarshalJSON       = errors.New("failed to marshal JSON")
)
