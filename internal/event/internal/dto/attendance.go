package dto

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Attendance is the `event_attendees` collection document: one player signed
// up for one event.
type Attendance struct {
	Id        bson.ObjectID `bson:"_id,omitempty"`
	EventId   string        `bson:"event_id"`
	UserId    string        `bson:"user_id"`
	CreatedAt time.Time     `bson:"created_at"`
	// RemindedFor is the start time the attendee was last reminded about. A
	// moved event gets a new start, so its attendees are reminded again.
	RemindedFor *time.Time `bson:"reminded_for,omitempty"`
}
