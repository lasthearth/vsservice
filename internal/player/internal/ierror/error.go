package ierror

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

var (
	ErrNotFound          = ierror.NotFound("user not found")
	ErrNickAlreadyExists = ierror.AlreadyExists("nickname already taken")
)
