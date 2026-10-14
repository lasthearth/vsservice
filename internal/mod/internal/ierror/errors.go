package ierror

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

var (
	// ErrModNotFound reports that ModDB knows no mod under the query.
	ErrModNotFound = ierror.NotFound("mod not found on ModDB")
	// ErrNotInList reports that the mod is not on the server's list.
	ErrNotInList = ierror.NotFound("mod is not in the list")
	// ErrUnavailable reports that ModDB did not answer.
	ErrUnavailable = ierror.Unavailable("ModDB did not answer")
	// ErrBadQuery reports a query that names no ModDB mod.
	ErrBadQuery = ierror.InvalidArgument("query is not a ModDB reference")
	// ErrUnknownVersion reports a version ModDB does not list for the mod.
	ErrUnknownVersion = ierror.InvalidArgument("ModDB has no such version")
	// ErrAudience reports that nobody said who needs the mod.
	ErrAudience = ierror.InvalidArgument("audience is unspecified")
	// ErrNote reports a note over the limit, or one carrying control characters.
	ErrNote = ierror.InvalidArgument("note must be at most 160 characters and free of control characters")
	// ErrAlreadyExists reports that the mod is already on the list.
	ErrAlreadyExists = ierror.AlreadyExists("the mod is already in the list")
	// ErrNoReleases reports a mod ModDB lists no releases for, so nothing can
	// say which version the server would run.
	ErrNoReleases = ierror.InvalidArgument("ModDB lists no releases for the mod")
	// ErrOrder reports an order that is not a permutation of the stored mods.
	ErrOrder = ierror.InvalidArgument("ids must list every mod exactly once")
)
