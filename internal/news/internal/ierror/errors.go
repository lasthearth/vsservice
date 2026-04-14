package ierror

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

var (
	ErrNotFound     = ierror.NotFound("not found")
	ErrNewsNotFound = ierror.NotFound("news not found")
)
