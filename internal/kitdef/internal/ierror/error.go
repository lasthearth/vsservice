package ierror

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

var (
	// ErrNotFound is returned when a kit code has never been captured by the
	// game. vsservice never inserts kits, so an unknown code is a NotFound.
	ErrNotFound = ierror.NotFound("kit not found")
)
