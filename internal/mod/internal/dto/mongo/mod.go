package dto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
)

// Mod is the server_mods collection document.
type Mod struct {
	// Embedded, not named: mongox.UpdateDoc finds the version guard on an
	// anonymous Model field.
	mongox.Model `bson:",inline"`

	// ModId is ModDB's modidstr. It is stored as the plain id field, which is
	// what the unique index and every lookup use.
	ModId    string `bson:"id"`
	Name     string `bson:"name"`
	Author   string `bson:"author"`
	LogoURL  string `bson:"logo_url,omitempty"`
	ModDBURL string `bson:"moddb_url,omitempty"`
	Side     string `bson:"side"`
	Audience string `bson:"audience"`
	// Source is where the record comes from: "server" or "admin". It is
	// omitted on documents written before the sync existed, which the model
	// classifies by audience.
	Source        string   `bson:"source,omitempty"`
	Note          string   `bson:"note,omitempty"`
	Summary       string   `bson:"summary,omitempty"`
	Release       *Release `bson:"release,omitempty"`
	LatestVersion string   `bson:"latest_version,omitempty"`
	Position      int      `bson:"position"`
	// Releases is the newest-first snapshot of what ModDB lists.
	Releases []Release `bson:"releases,omitempty"`
}

// Release is one version of a mod inside a stored document.
type Release struct {
	Version      string    `bson:"version"`
	GameVersions []string  `bson:"game_versions,omitempty"`
	FileURL      string    `bson:"file_url"`
	FileName     string    `bson:"file_name"`
	FileSize     int64     `bson:"file_size"`
	SHA256       string    `bson:"sha256,omitempty"`
	FileKey      string    `bson:"file_key,omitempty"`
	ReleasedAt   time.Time `bson:"released_at"`
}
