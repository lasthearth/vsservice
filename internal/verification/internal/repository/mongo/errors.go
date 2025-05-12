package repository

import "github.com/go-faster/errors"

var (
	ErrCreate   = errors.New("failed to create stats")
	ErrNotFound = errors.New("stats not found")
	ErrUpdate   = errors.New("failed to update stats")
)
