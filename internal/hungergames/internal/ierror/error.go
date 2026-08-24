package ierror

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

var (
	ErrNotFound           = ierror.NotFound("not found")
	ErrNoActiveSeason     = ierror.NotFound("no active season")
	ErrActiveSeasonExists = ierror.AlreadyExists("active season already exists")
	// ErrSeasonAlreadyClosed reports that a concurrent caller closed the season
	// first. It is what makes CloseSeason usable as a claim: only one caller can
	// win, so the rewards are paid exactly once.
	ErrSeasonAlreadyClosed = ierror.AlreadyExists("season is already closed")
)
