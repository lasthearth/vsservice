package model

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	// SubjectMinLen and SubjectMaxLen bound the subject, in characters.
	SubjectMinLen = 3
	SubjectMaxLen = 120
	// TextMaxLen bounds one message, in characters.
	TextMaxLen = 4000
	// MaxMessages caps one thread, so the ticket document stays small.
	MaxMessages = 200
	// MaxActivePerAuthor is how many tickets not closed one player may keep.
	MaxActivePerAuthor = 3
	// AutoCloseAfter is how long an ANSWERED ticket waits for the player
	// before it closes itself.
	AutoCloseAfter = 7 * 24 * time.Hour
)

// Errors returned by the model. The service maps them to typed domain errors.
var (
	ErrAuthorRequired   = errors.New("author is required")
	ErrCategoryInvalid  = errors.New("unknown category")
	ErrStatusInvalid    = errors.New("unknown status")
	ErrSubjectInvalid   = errors.New("subject must be 3 to 120 characters")
	ErrTextInvalid      = errors.New("text must be 1 to 4000 characters")
	ErrClosed           = errors.New("the ticket is closed")
	ErrNotClosed        = errors.New("the ticket is not closed")
	ErrThreadFull       = errors.New("the ticket holds too many messages")
	ErrCloseWithNote    = errors.New("an internal note cannot close the ticket")
	ErrNotAuthor        = errors.New("not the author of the ticket")
	ErrAssigneeRequired = errors.New("assignee is required")
)

// Category is what a ticket is about.
type Category string

const (
	CategoryBug        Category = "bug"
	CategoryComplaint  Category = "complaint"
	CategoryDonate     Category = "donate"
	CategorySettlement Category = "settlement"
	CategoryQuestion   Category = "question"
	CategoryOther      Category = "other"
)

// IsValid reports whether c is a known category.
func (c Category) IsValid() bool {
	switch c {
	case CategoryBug, CategoryComplaint, CategoryDonate, CategorySettlement, CategoryQuestion, CategoryOther:
		return true
	default:
		return false
	}
}

// Status is who the ticket waits for.
type Status string

const (
	// StatusOpen waits for the staff.
	StatusOpen Status = "open"
	// StatusAnswered waits for the player.
	StatusAnswered Status = "answered"
	// StatusClosed is done.
	StatusClosed Status = "closed"
)

// IsValid reports whether s is a known status.
func (s Status) IsValid() bool {
	return s == StatusOpen || s == StatusAnswered || s == StatusClosed
}

// Message is one entry of a ticket thread.
type Message struct {
	Id       string
	AuthorId string
	// Staff tells a staff reply or note from the player's message.
	Staff bool
	Text  string
	// Internal marks a note only the staff can see.
	Internal  bool
	CreatedAt time.Time
}

// Ticket is a support request and its thread.
type Ticket struct {
	Id         string
	AuthorId   string
	Subject    string
	Category   Category
	Status     Status
	AssigneeId string
	// Messages is the thread, oldest first. Lists load tickets without it;
	// PublicCount and MessageCount still tell its size then.
	Messages []Message
	// PublicCount is how many messages the author can see, MessageCount the
	// whole thread with internal notes.
	PublicCount  int32
	MessageCount int32
	// AuthorUnread is set by a staff reply and cleared when the author reads
	// the ticket; StaffUnread the other way round.
	AuthorUnread bool
	StaffUnread  bool
	// LastPublicAt is the last message the author can see; LastMessageAt the
	// last message of all, internal notes included. The player's list sorts by
	// the first, the staff inbox by the second.
	LastPublicAt  time.Time
	LastMessageAt time.Time
	// AnsweredAt is when the ticket last moved to ANSWERED; auto-close counts
	// from it.
	AnsweredAt *time.Time
	ClosedAt   *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewTicket validates the request and builds an OPEN ticket with its first
// message.
func NewTicket(authorId, subject string, category Category, text string, now time.Time) (*Ticket, error) {
	if strings.TrimSpace(authorId) == "" {
		return nil, ErrAuthorRequired
	}
	if !category.IsValid() {
		return nil, ErrCategoryInvalid
	}
	subject = strings.TrimSpace(subject)
	if n := utf8.RuneCountInString(subject); n < SubjectMinLen || n > SubjectMaxLen {
		return nil, ErrSubjectInvalid
	}
	text, err := cleanText(text)
	if err != nil {
		return nil, err
	}

	t := &Ticket{
		AuthorId:      authorId,
		Subject:       subject,
		Category:      category,
		Status:        StatusOpen,
		Messages:      []Message{newMessage(authorId, false, text, false, now)},
		StaffUnread:   true,
		LastPublicAt:  now,
		LastMessageAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	t.recount()
	return t, nil
}

// IsAuthor reports whether userId opened the ticket.
func (t *Ticket) IsAuthor(userId string) bool {
	return userId != "" && userId == t.AuthorId
}

// IsClosed reports whether the ticket is closed.
func (t *Ticket) IsClosed() bool {
	return t.Status == StatusClosed
}

// PostByAuthor adds the author's message and hands the ticket back to the
// staff.
func (t *Ticket) PostByAuthor(userId, text string, now time.Time) error {
	if !t.IsAuthor(userId) {
		return ErrNotAuthor
	}
	if t.IsClosed() {
		return ErrClosed
	}
	text, err := cleanText(text)
	if err != nil {
		return err
	}
	if t.MessageCount >= MaxMessages {
		return ErrThreadFull
	}

	t.Messages = append(t.Messages, newMessage(userId, false, text, false, now))
	t.recount()
	t.Status = StatusOpen
	t.AnsweredAt = nil
	t.StaffUnread = true
	t.LastPublicAt = now
	t.LastMessageAt = now
	return nil
}

// ReplyByStaff adds a staff reply, or an internal note when internal is set.
//
// A reply hands the ticket to the player (or closes it with closeAfter) and
// reopens a closed one; a note leaves the status alone. The replying admin
// takes a ticket nobody took.
func (t *Ticket) ReplyByStaff(staffId, text string, internal, closeAfter bool, now time.Time) error {
	if strings.TrimSpace(staffId) == "" {
		return ErrAuthorRequired
	}
	if internal && closeAfter {
		return ErrCloseWithNote
	}
	text, err := cleanText(text)
	if err != nil {
		return err
	}
	if t.MessageCount >= MaxMessages {
		return ErrThreadFull
	}

	t.Messages = append(t.Messages, newMessage(staffId, true, text, internal, now))
	t.recount()
	t.LastMessageAt = now
	if t.AssigneeId == "" {
		t.AssigneeId = staffId
	}
	if internal {
		return nil
	}

	t.LastPublicAt = now
	t.AuthorUnread = true
	t.StaffUnread = false
	if closeAfter {
		t.close(now)
		return nil
	}
	t.ClosedAt = nil
	t.Status = StatusAnswered
	t.AnsweredAt = &now
	return nil
}

// CloseByAuthor closes the ticket at the author's request. Closing a closed
// ticket keeps the first moment.
func (t *Ticket) CloseByAuthor(userId string, now time.Time) error {
	if !t.IsAuthor(userId) {
		return ErrNotAuthor
	}
	t.close(now)
	return nil
}

// ReopenByAuthor hands a closed ticket back to the staff.
func (t *Ticket) ReopenByAuthor(userId string, now time.Time) error {
	if !t.IsAuthor(userId) {
		return ErrNotAuthor
	}
	if !t.IsClosed() {
		return ErrNotClosed
	}
	t.Status = StatusOpen
	t.ClosedAt = nil
	t.AnsweredAt = nil
	t.StaffUnread = true
	t.LastPublicAt = now
	t.LastMessageAt = now
	return nil
}

// SetStatus is the staff's manual status change.
func (t *Ticket) SetStatus(status Status, now time.Time) error {
	if !status.IsValid() {
		return ErrStatusInvalid
	}
	switch status {
	case StatusClosed:
		t.close(now)
	case StatusAnswered:
		t.ClosedAt = nil
		if t.Status != StatusAnswered {
			t.AnsweredAt = &now
		}
		t.Status = StatusAnswered
	case StatusOpen:
		t.ClosedAt = nil
		t.AnsweredAt = nil
		t.Status = StatusOpen
	}
	return nil
}

// SetCategory moves the ticket to another category.
func (t *Ticket) SetCategory(category Category) error {
	if !category.IsValid() {
		return ErrCategoryInvalid
	}
	t.Category = category
	return nil
}

// Assign gives the ticket to staffId; an empty id releases it.
func (t *Ticket) Assign(staffId string) {
	t.AssigneeId = strings.TrimSpace(staffId)
}

// MarkReadByAuthor clears the author's unread mark. Reports whether it was
// set, so a read that changes nothing is not written.
func (t *Ticket) MarkReadByAuthor() bool {
	was := t.AuthorUnread
	t.AuthorUnread = false
	return was
}

// MarkReadByStaff clears the staff unread mark. Reports whether it was set.
func (t *Ticket) MarkReadByStaff() bool {
	was := t.StaffUnread
	t.StaffUnread = false
	return was
}

// AutoClose closes an ANSWERED ticket the player left quiet for
// AutoCloseAfter. Reports whether it closed the ticket.
func (t *Ticket) AutoClose(now time.Time) bool {
	if t.Status != StatusAnswered || t.AnsweredAt == nil || now.Sub(*t.AnsweredAt) < AutoCloseAfter {
		return false
	}
	t.close(now)
	return true
}

// PublicMessages are the messages the author can see, oldest first.
func (t *Ticket) PublicMessages() []Message {
	out := make([]Message, 0, len(t.Messages))
	for _, m := range t.Messages {
		if !m.Internal {
			out = append(out, m)
		}
	}
	return out
}

// Touch stamps the persisted update time (called by mongox.UpdateDoc).
func (t *Ticket) Touch(now time.Time) { t.UpdatedAt = now }

func (t *Ticket) close(now time.Time) {
	if t.Status == StatusClosed && t.ClosedAt != nil {
		return
	}
	t.Status = StatusClosed
	t.ClosedAt = &now
	t.AnsweredAt = nil
}

func (t *Ticket) recount() {
	t.MessageCount = int32(len(t.Messages))
	t.PublicCount = 0
	for _, m := range t.Messages {
		if !m.Internal {
			t.PublicCount++
		}
	}
}

func newMessage(authorId string, staff bool, text string, internal bool, now time.Time) Message {
	return Message{
		Id:        bson.NewObjectIDFromTimestamp(now).Hex(),
		AuthorId:  authorId,
		Staff:     staff,
		Text:      text,
		Internal:  internal,
		CreatedAt: now,
	}
}

func cleanText(text string) (string, error) {
	text = strings.TrimSpace(text)
	if n := utf8.RuneCountInString(text); n == 0 || n > TextMaxLen {
		return "", ErrTextInvalid
	}
	return text, nil
}
