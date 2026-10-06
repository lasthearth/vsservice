package dto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
)

// Event is the `events` collection document.
type Event struct {
	Model       mongox.Model `bson:",inline"`
	Title       string       `bson:"title"`
	Description string       `bson:"description"`
	Cover       string       `bson:"cover,omitempty"`
	Location    string       `bson:"location,omitempty"`
	StartsAt    time.Time    `bson:"starts_at"`
	EndsAt      *time.Time   `bson:"ends_at,omitempty"`
	// Until is EndsAt, or StartsAt when there is no end. Denormalized so the
	// upcoming/past split is a single indexed range query.
	Until     time.Time  `bson:"until"`
	CreatedBy string     `bson:"created_by"`
	DeletedAt *time.Time `bson:"deleted_at,omitempty"`
	DeletedBy string     `bson:"deleted_by,omitempty"`
}
