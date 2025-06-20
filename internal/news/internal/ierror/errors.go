// ierror provides custom error types for the news domain.
package ierror

import "errors"

var (
	// ErrNotFound is returned when a single resource entry is not found.
	ErrNotFound     = errors.New("not found")
	ErrNewsNotFound = errors.New("news not found")
)
