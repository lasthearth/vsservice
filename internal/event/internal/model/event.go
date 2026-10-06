package model

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lasthearth/vsservice/internal/pkg/ierror"
)

const (
	// TitleMaxLen is the longest title, in characters.
	TitleMaxLen = 120
	// LocationMaxLen is the longest location, in characters.
	LocationMaxLen = 120
	// DescriptionMaxLen is the longest description, in characters.
	DescriptionMaxLen = 20000
)

var (
	ErrTitleRequired   = ierror.InvalidArgument("title is required")
	ErrTitleTooLong    = ierror.InvalidArgument("title is too long")
	ErrLocationTooLong = ierror.InvalidArgument("location is too long")
	ErrDescriptionLong = ierror.InvalidArgument("description is too long")
	ErrStartRequired   = ierror.InvalidArgument("starts_at is required")
	ErrTimeOutOfRange  = ierror.InvalidArgument("event time is out of the supported range")
	ErrEndBeforeStart  = ierror.InvalidArgument("ends_at must not be before starts_at")
	ErrInvalidCoverURL = ierror.InvalidArgument("invalid cover url")
	ErrCreatorRequired = ierror.InvalidArgument("created_by is required")
)

// The plausible window for an event's times.
//
// Lower bound: a present-but-empty protobuf Timestamp converts to the Unix
// epoch, not to a nil pointer, and IsZero only matches year 1 — so "was a value
// supplied at all" has to be a bound, not a zero check. Without it a 1970 event
// is stored with a derived until that never satisfies the upcoming filter, and
// it stays invisible forever.
//
// Upper bound: timestamppb.AsTime is best effort, and seconds near
// math.MaxInt64 overflow the driver's Unix()*1000 conversion into a corrupt
// 1969 datetime.
var (
	minTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	maxTime = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
)

// inWindow reports whether t is a plausible event time.
func inWindow(t time.Time) bool {
	return !t.Before(minTime) && !t.After(maxTime)
}

// Details are the fields an editor sets on an event.
type Details struct {
	Title       string
	Description string
	Cover       string
	Location    string
	StartsAt    time.Time
	EndsAt      *time.Time
}

// Event is a calendar entry: a wipe, a siege, a fair, a tournament.
type Event struct {
	Id          string
	Title       string
	Description string
	Cover       string
	Location    string
	StartsAt    time.Time
	EndsAt      *time.Time
	CreatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// New builds an event from validated details.
func New(details Details, createdBy string) (*Event, error) {
	if strings.TrimSpace(createdBy) == "" {
		return nil, ErrCreatorRequired
	}

	e := &Event{CreatedBy: createdBy}
	if err := e.Apply(details); err != nil {
		return nil, err
	}

	return e, nil
}

// Apply validates the details and replaces the editable fields with them.
// Nothing changes when validation fails.
func (e *Event) Apply(d Details) error {
	title := strings.TrimSpace(d.Title)
	location := strings.TrimSpace(d.Location)

	switch {
	case title == "":
		return ErrTitleRequired
	case utf8.RuneCountInString(title) > TitleMaxLen:
		return ErrTitleTooLong
	case utf8.RuneCountInString(location) > LocationMaxLen:
		return ErrLocationTooLong
	case utf8.RuneCountInString(d.Description) > DescriptionMaxLen:
		return ErrDescriptionLong
	case d.StartsAt.IsZero():
		return ErrStartRequired
	case !inWindow(d.StartsAt):
		return ErrTimeOutOfRange
	case d.EndsAt != nil && !inWindow(*d.EndsAt):
		return ErrTimeOutOfRange
	case d.EndsAt != nil && d.EndsAt.Before(d.StartsAt):
		return ErrEndBeforeStart
	}

	e.Title = title
	e.Description = d.Description
	e.Cover = strings.TrimSpace(d.Cover)
	e.Location = location
	e.StartsAt = d.StartsAt.UTC()
	e.EndsAt = nil
	if d.EndsAt != nil {
		end := d.EndsAt.UTC()
		e.EndsAt = &end
	}

	return nil
}

// DefaultDuration is how long an event without an end counts as running: it
// stays in the upcoming list (shown as "happening now") for this long.
const DefaultDuration = 2 * time.Hour

// Until is the moment the event leaves the upcoming list: its end, or
// DefaultDuration after its start when no end is set.
func (e *Event) Until() time.Time {
	if e.EndsAt != nil {
		return *e.EndsAt
	}
	return e.StartsAt.Add(DefaultDuration)
}

// Touch stamps the persisted update time. mongox.UpdateDoc calls it, so a
// caller sees the value that was actually written rather than the one it read.
func (e *Event) Touch(t time.Time) { e.UpdatedAt = t }
