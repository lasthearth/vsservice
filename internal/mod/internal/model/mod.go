package model

import "time"

// Audience is who needs the mod, as the site tells it.
type Audience uint8

const (
	AudienceUnspecified Audience = iota
	// AudienceRequired marks a mod the game will not join the server without.
	AudienceRequired
	// AudienceOptional is a client-side extra players may install.
	AudienceOptional
	// AudienceHidden is server-only: never shown to players, never archived.
	AudienceHidden
)

// Side is where the mod runs, as ModDB states it.
type Side uint8

const (
	SideUnspecified Side = iota
	SideBoth
	SideClient
	SideServer
)

const (
	// MaxNoteRunes bounds the staff's Russian one-liner.
	MaxNoteRunes = 160
	// MaxSnapshotRunes bounds the description taken from ModDB.
	MaxSnapshotRunes = 200
	// MaxReleases is how many of a mod's releases a stored mod keeps.
	MaxReleases = 15
)

// Release is one version of a mod, as it was when the mod was read.
type Release struct {
	Version      string
	GameVersions []string
	FileURL      string
	FileName     string
	FileSize     int64
	ReleasedAt   time.Time
}

// Snapshot is everything ModDB says about a mod: the part the staff does not
// decide.
type Snapshot struct {
	Name     string
	Author   string
	LogoURL  string
	ModDBURL string
	Side     Side
	Summary  string
	Releases []Release
}

// Mod is one mod of the game server.
type Mod struct {
	// ModId is ModDB's modidstr, e.g. "carryon". It is the id the API serves and
	// the document is looked up by.
	ModId    string
	Name     string
	Author   string
	LogoURL  string
	ModDBURL string
	Side     Side
	Audience Audience
	Note     string
	Summary  string
	// Release is the version the server runs; nil until one is chosen.
	Release *Release
	// LatestVersion is the newest version ModDB lists; empty when Release is
	// already the newest.
	LatestVersion string
	// Position is the display order: 0 is the first mod.
	Position int
	// UpdatedAt is the stored write time, which the archive's timestamp reports.
	UpdatedAt time.Time
	// Releases is the newest-first snapshot of what ModDB lists. It is served to
	// staff only.
	Releases []Release
}

// Build assembles a mod from what ModDB says and what the staff decided.
// chosen is the release the server runs; nil means none was picked yet, which
// is what a lookup answers with.
func Build(
	modId string,
	snapshot Snapshot,
	audience Audience,
	note string,
	chosen *Release,
	now time.Time,
) Mod {
	mod := Mod{
		ModId:    modId,
		Name:     snapshot.Name,
		Author:   snapshot.Author,
		LogoURL:  snapshot.LogoURL,
		ModDBURL: snapshot.ModDBURL,
		Side:     snapshot.Side,
		Audience: audience,
		Note:     note,
		Summary:  snapshot.Summary,
		Releases: snapshot.Releases,
		UpdatedAt: now,
	}

	mod.LatestVersion = LatestVersion(snapshot.Releases, versionOf(chosen))
	mod.Release = chosen

	return mod
}

// versionOf is the version a release names; empty when none is chosen.
func versionOf(release *Release) string {
	if release == nil {
		return ""
	}

	return release.Version
}

// Version is the version the server runs; empty when none was chosen.
func (m *Mod) Version() string {
	if m.Release == nil {
		return ""
	}

	return m.Release.Version
}

// SetFileSize records how large the release's file is; 0 means unknown.
func (r *Release) SetFileSize(size int64) { r.FileSize = size }

// SetAudience records who needs the mod.
func (m *Mod) SetAudience(audience Audience) { m.Audience = audience }

// SetNote stores the staff's one-liner; the caller has already checked it.
func (m *Mod) SetNote(note string) { m.Note = note }

// SetRelease makes release the version the server runs and keeps
// LatestVersion in step with it.
func (m *Mod) SetRelease(release Release) {
	m.Release = &release
	m.LatestVersion = LatestVersion(m.Releases, release.Version)
}

// SetPosition records where the mod stands in the list.
func (m *Mod) SetPosition(position int) { m.Position = position }

// SetUpdatedAt stamps the mod with what the stored document says, which a
// mapper that ignores the Mongo envelope cannot carry.
func (m *Mod) SetUpdatedAt(at time.Time) { m.UpdatedAt = at }

// Touch stamps the write. mongox.UpdateDoc calls it, so the caller of an update
// sees the persisted timestamp.
func (m *Mod) Touch(at time.Time) { m.UpdatedAt = at }

// SetSnapshot replaces everything ModDB says about the mod. What the staff
// decided — the audience, the note and the version the server runs — stays.
//
// The chosen release is re-pointed at the same version inside the new list, so
// a refresh can never silently move the server to another version; when ModDB
// no longer lists it, the stored release is kept as it was.
func (m *Mod) SetSnapshot(snapshot Snapshot) {
	m.Name = snapshot.Name
	m.Author = snapshot.Author
	m.LogoURL = snapshot.LogoURL
	m.ModDBURL = snapshot.ModDBURL
	m.Side = snapshot.Side
	m.Summary = snapshot.Summary
	m.Releases = snapshot.Releases

	if version := m.Version(); version != "" {
		if fresh, ok := FindRelease(snapshot.Releases, version); ok {
			m.Release = &fresh
		}
	}

	m.LatestVersion = LatestVersion(snapshot.Releases, m.Version())
}

// LatestVersion is the newest release's version, or "" when chosen is already
// the newest one. No releases at all is no version to offer.
func LatestVersion(releases []Release, chosen string) string {
	if len(releases) == 0 {
		return ""
	}

	newest := releases[0].Version
	if newest == chosen {
		return ""
	}

	return newest
}

// FindRelease returns the release of one version, if ModDB still lists it.
func FindRelease(releases []Release, version string) (Release, bool) {
	for _, release := range releases {
		if release.Version == version {
			return release, true
		}
	}

	return Release{}, false
}
