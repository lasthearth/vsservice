package model

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewTrimsAndNormalizes(t *testing.T) {
	msk := time.FixedZone("MSK", 3*60*60)
	start := time.Date(2026, 10, 10, 18, 0, 0, 0, msk)

	e, err := New(Details{Title: "  Осада  ", Location: " Форт ", StartsAt: start}, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if e.Title != "Осада" || e.Location != "Форт" {
		t.Errorf("fields not trimmed: %q %q", e.Title, e.Location)
	}
	if e.StartsAt.Location() != time.UTC || !e.StartsAt.Equal(start) {
		t.Errorf("starts_at must be the same instant in UTC, got %v", e.StartsAt)
	}
	if want := start.Add(DefaultDuration); !e.Until().Equal(want) {
		t.Errorf("until without end must be start + DefaultDuration, got %v", e.Until())
	}
}

func TestApplyRejects(t *testing.T) {
	start := time.Date(2026, 10, 10, 18, 0, 0, 0, time.UTC)
	before := start.Add(-time.Hour)
	// What a present-but-empty protobuf Timestamp converts to. IsZero does not
	// catch it, so without the window check a 1970 event would be stored with a
	// derived until that never satisfies the upcoming filter.
	epoch := time.Unix(0, 0).UTC()
	// Past the upper bound: seconds near MaxInt64 overflow the driver's
	// Unix()*1000 conversion into a corrupt 1969 datetime.
	farFuture := time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		d    Details
		want error
	}{
		{"empty title", Details{Title: "   ", StartsAt: start}, ErrTitleRequired},
		{"long title", Details{Title: strings.Repeat("я", TitleMaxLen+1), StartsAt: start}, ErrTitleTooLong},
		{"long location", Details{Title: "t", Location: strings.Repeat("x", LocationMaxLen+1), StartsAt: start}, ErrLocationTooLong},
		{"no start", Details{Title: "t"}, ErrStartRequired},
		{"epoch start", Details{Title: "t", StartsAt: epoch}, ErrTimeOutOfRange},
		{"far future start", Details{Title: "t", StartsAt: farFuture}, ErrTimeOutOfRange},
		{"far future end", Details{Title: "t", StartsAt: start, EndsAt: &farFuture}, ErrTimeOutOfRange},
		{"end before start", Details{Title: "t", StartsAt: start, EndsAt: &before}, ErrEndBeforeStart},
	}
	for _, c := range cases {
		e := &Event{Title: "kept"}
		if err := e.Apply(c.d); !errors.Is(err, c.want) {
			t.Errorf("%s: want %v, got %v", c.name, c.want, err)
		}
		if e.Title != "kept" {
			t.Errorf("%s: event changed on failed validation", c.name)
		}
	}
}

func TestNewRequiresCreator(t *testing.T) {
	_, err := New(Details{Title: "t", StartsAt: time.Now()}, " ")
	if !errors.Is(err, ErrCreatorRequired) {
		t.Errorf("want ErrCreatorRequired, got %v", err)
	}
}

func TestUntilUsesEnd(t *testing.T) {
	start := time.Date(2026, 10, 10, 18, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	e, err := New(Details{Title: "t", StartsAt: start, EndsAt: &end}, "u")
	if err != nil {
		t.Fatal(err)
	}
	if !e.Until().Equal(end) {
		t.Errorf("want until = end, got %v", e.Until())
	}
}

// TestTouchStampsPersistedTime pins the hook mongox.UpdateDoc looks for: without
// it the model returned to the caller carries the updated_at it was read with,
// not the one that was written.
func TestTouchStampsPersistedTime(t *testing.T) {
	stamp := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)

	e := &Event{}
	e.Touch(stamp)

	if !e.UpdatedAt.Equal(stamp) {
		t.Errorf("Touch did not stamp updated_at: %v", e.UpdatedAt)
	}
}
