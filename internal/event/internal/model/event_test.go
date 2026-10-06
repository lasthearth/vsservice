package model

import (
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

	cases := []struct {
		name string
		d    Details
		want error
	}{
		{"empty title", Details{Title: "   ", StartsAt: start}, ErrTitleRequired},
		{"long title", Details{Title: strings.Repeat("я", TitleMaxLen+1), StartsAt: start}, ErrTitleTooLong},
		{"long location", Details{Title: "t", Location: strings.Repeat("x", LocationMaxLen+1), StartsAt: start}, ErrLocationTooLong},
		{"no start", Details{Title: "t"}, ErrStartRequired},
		{"end before start", Details{Title: "t", StartsAt: start, EndsAt: &before}, ErrEndBeforeStart},
	}
	for _, c := range cases {
		e := &Event{Title: "kept"}
		if err := e.Apply(c.d); err != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, err)
		}
		if e.Title != "kept" {
			t.Errorf("%s: event changed on failed validation", c.name)
		}
	}
}

func TestNewRequiresCreator(t *testing.T) {
	_, err := New(Details{Title: "t", StartsAt: time.Now()}, " ")
	if err != ErrCreatorRequired {
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
