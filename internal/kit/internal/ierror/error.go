package ierror

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

var (
	ErrEmptyUid = ierror.InvalidArgument("user uid is empty")
	ErrNotFound = ierror.NotFound("kit not found")
)
