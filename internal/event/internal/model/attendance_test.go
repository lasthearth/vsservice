package model

import (
	"errors"
	"testing"
	"time"
)

var attendNow = time.Date(2026, 10, 10, 18, 0, 0, 0, time.UTC)

func eventAt(t *testing.T, start time.Time, end *time.Time) *Event {
	t.Helper()
	e, err := New(Details{Title: "Ярмарка", StartsAt: start, EndsAt: end}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestAcceptsAttendanceUntilEnd(t *testing.T) {
	e := eventAt(t, attendNow.Add(time.Hour), nil)

	if err := e.AcceptsAttendance(attendNow); err != nil {
		t.Fatalf("upcoming: %v", err)
	}
	// Running events still take sign-ups: people join a fair that has begun.
	if err := e.AcceptsAttendance(attendNow.Add(2 * time.Hour)); err != nil {
		t.Fatalf("ongoing: %v", err)
	}
	if err := e.AcceptsAttendance(e.Until()); !errors.Is(err, ErrEventOver) {
		t.Fatalf("at the end: want ErrEventOver, got %v", err)
	}
}

func TestReminderDueWindow(t *testing.T) {
	e := eventAt(t, attendNow.Add(ReminderLead), nil)

	cases := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"too early", attendNow.Add(-time.Minute), false},
		{"exactly an hour before", attendNow, true},
		{"minutes before", e.StartsAt.Add(-5 * time.Minute), true},
		{"at the start", e.StartsAt, false},
		{"after the start", e.StartsAt.Add(time.Minute), false},
	}
	for _, c := range cases {
		if got := e.ReminderDue(c.now); got != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
}
