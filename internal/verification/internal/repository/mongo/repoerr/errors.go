package repoerr

import "github.com/go-faster/errors"

var (
	ErrCreate   = errors.New("failed to create verification")
	ErrNotFound = errors.New("verification not found")
	ErrUpdate   = errors.New("failed to update stats")
)
