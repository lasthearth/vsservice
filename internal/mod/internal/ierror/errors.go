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
	// ErrServerMod reports an edit of a mod the game server's manifest owns.
	// The sync rewrites its version and audience, so the staff may only change
	// its note, and deleting it would only remove it until the next sync.
	ErrServerMod = ierror.InvalidArgument("required mods come from the server")
	// ErrNoReleases reports a mod ModDB lists no releases for, so nothing can
	// say which version the server would run.
	ErrNoReleases = ierror.InvalidArgument("ModDB lists no releases for the mod")
	// ErrOrder reports an order that is not a permutation of the stored mods.
	ErrOrder = ierror.InvalidArgument("ids must list every mod exactly once")
	// ErrNoModId reports a mod ModDB states no id for, so there is nothing to
	// store it under.
	ErrNoModId = ierror.InvalidArgument("ModDB names no id for the mod")
	// ErrNoUsableFile reports a mod whose files are none of them on a host this
	// service may fetch.
	ErrNoUsableFile = ierror.InvalidArgument("ModDB names no file this service may fetch")
	// ErrAssetIDRef reports a ModDB page link. Its number is an asset id, and
	// the API resolves a mod id, so the same digits name two different mods.
	ErrAssetIDRef = ierror.InvalidArgument(
		"a ModDB page link carries an asset id, which the API cannot resolve: " +
			"paste the mod's short link (mods.vintagestory.at/<alias>) or its numeric mod id")
)
