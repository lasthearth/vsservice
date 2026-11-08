package dto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
)

// Message is one entry of a ticket thread, embedded in Ticket.
type Message struct {
	Id        string    `bson:"id"`
	AuthorId  string    `bson:"author_id"`
	Staff     bool      `bson:"staff,omitempty"`
	Text      string    `bson:"text"`
	Internal  bool      `bson:"internal,omitempty"`
	CreatedAt time.Time `bson:"created_at"`
}

// Ticket is the `tickets` collection document. The thread is embedded: it is
// capped at model.MaxMessages and always read and written with its ticket.
type Ticket struct {
	// Embedded, not named: mongox.UpdateDoc finds the version guard on an
	// anonymous Model field.
	mongox.Model `bson:",inline"`

	AuthorId      string     `bson:"author_id"`
	Subject       string     `bson:"subject"`
	Category      string     `bson:"category"`
	Status        string     `bson:"status"`
	AssigneeId    string     `bson:"assignee_id"`
	Messages      []Message  `bson:"messages"`
	AuthorUnread  bool       `bson:"author_unread"`
	StaffUnread   bool       `bson:"staff_unread"`
	LastPublicAt  time.Time  `bson:"last_public_at"`
	LastMessageAt time.Time  `bson:"last_message_at"`
	AnsweredAt    *time.Time `bson:"answered_at,omitempty"`
	ClosedAt      *time.Time `bson:"closed_at,omitempty"`
	// PublicCount is how many messages the author can see. Kept beside the
	// thread so lists can show it without loading the messages.
	PublicCount int32 `bson:"public_count"`
	// MessageCount is the whole thread, internal notes included.
	MessageCount int32 `bson:"message_count"`
}

// SavedReply is the `ticket_saved_replies` collection document.
type SavedReply struct {
	mongox.Model `bson:",inline"`

	Title     string `bson:"title"`
	Text      string `bson:"text"`
	UpdatedBy string `bson:"updated_by"`
}
