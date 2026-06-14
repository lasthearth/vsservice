package model

import (
	"testing"
	"time"
)

func TestNewSeason(t *testing.T) {
	before := time.Now()
	s := NewSeason(3)
	after := time.Now()

	if s.Number != 3 {
		t.Errorf("Number = %d, want 3", s.Number)
	}
	if s.EndedAt != nil {
		t.Errorf("EndedAt should be nil on creation")
	}
	if s.StartedAt.Before(before) || s.StartedAt.After(after) {
		t.Errorf("StartedAt out of expected range")
	}
}

func TestSeason_IsActive(t *testing.T) {
	tests := []struct {
		name    string
		endedAt *time.Time
		want    bool
	}{
		{"nil EndedAt — active", nil, true},
		{"set EndedAt — not active", func() *time.Time { t := time.Now(); return &t }(), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &Season{EndedAt: tc.endedAt}
			if got := s.IsActive(); got != tc.want {
				t.Errorf("IsActive() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSeason_End(t *testing.T) {
	s := NewSeason(1)

	if !s.IsActive() {
		t.Fatal("season should be active before End()")
	}

	before := time.Now()
	s.End()
	after := time.Now()

	if s.IsActive() {
		t.Error("season should not be active after End()")
	}
	if s.EndedAt == nil {
		t.Fatal("EndedAt should be set after End()")
	}
	if s.EndedAt.Before(before) || s.EndedAt.After(after) {
		t.Errorf("EndedAt out of expected range")
	}
}

func TestSeason_End_Idempotent(t *testing.T) {
	s := NewSeason(1)
	s.End()

	time.Sleep(time.Millisecond)
	s.End() // second End() should not panic
}
