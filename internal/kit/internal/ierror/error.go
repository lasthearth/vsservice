package ierror

import "errors"

var (
	ErrEmptyUid = errors.New("user")
	ErrNotFound = errors.New("kit not found")
)
