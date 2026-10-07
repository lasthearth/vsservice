package dto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
)

// Post is the `lfg_posts` collection document.
type Post struct {
	// Embedded, not named: mongox.UpdateDoc finds the version guard on an
	// anonymous Model field.
	mongox.Model `bson:",inline"`

	AuthorId    string     `bson:"author_id"`
	Kind        string     `bson:"kind"`
	Activities  []string   `bson:"activities"`
	Title       string     `bson:"title"`
	Description string     `bson:"description,omitempty"`
	StartsAt    *time.Time `bson:"starts_at,omitempty"`
	Slots       int32      `bson:"slots"`
	Responders  []string   `bson:"responders"`
	PlayDays    []int32    `bson:"play_days,omitempty"`
	PlayTimes   []string   `bson:"play_times,omitempty"`
	Experience  string     `bson:"experience,omitempty"`
	Voice       bool       `bson:"voice,omitempty"`
	Contact     string     `bson:"contact,omitempty"`
	ClosedAt    *time.Time `bson:"closed_at,omitempty"`
	ExpiresAt   time.Time  `bson:"expires_at"`
	BumpedAt    time.Time  `bson:"bumped_at"`
	// DeleteAt drives the TTL index: a post is kept a day after it leaves the
	// board, so shared links still show what it was and a teammate post can
	// still be renewed.
	DeleteAt time.Time `bson:"delete_at"`
}
