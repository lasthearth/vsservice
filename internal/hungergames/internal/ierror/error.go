package ierror

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

var (
	ErrNotFound      = ierror.NotFound("not found")
	ErrNoActiveSeason = ierror.NotFound("no active season")
	ErrActiveSeasonExists = ierror.AlreadyExists("active season already exists")
)
