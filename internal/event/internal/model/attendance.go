package model

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/ierror"
)

const (
	// ReminderLead is how long before the start attendees are reminded.
	ReminderLead = time.Hour
	// AttendeePreviewSize is how many attendees an event card shows as avatars.
	AttendeePreviewSize = 5
)

// ErrEventOver refuses sign-ups for an event that has ended.
var ErrEventOver = ierror.FailedPrecondition("event is over")

// Attendees summarizes who signed up for an event.
type Attendees struct {
	Count int32
	// Preview holds the first AttendeePreviewSize attendees, in sign-up order.
	Preview []string
}

// AcceptsAttendance reports whether players may still sign up or withdraw:
// until the event leaves the upcoming list. A finished event is history.
func (e *Event) AcceptsAttendance(now time.Time) error {
	if !now.Before(e.Until()) {
		return ErrEventOver
	}
	return nil
}

// ReminderDue reports whether now falls in the reminder window: the event has
// not started yet and starts within ReminderLead.
func (e *Event) ReminderDue(now time.Time) bool {
	return now.Before(e.StartsAt) && !e.StartsAt.After(now.Add(ReminderLead))
}
