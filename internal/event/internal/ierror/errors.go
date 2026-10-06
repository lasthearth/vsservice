package ierror

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

// ErrNotFound is returned when an event does not exist or was deleted.
var ErrNotFound = ierror.NotFound("event not found")
