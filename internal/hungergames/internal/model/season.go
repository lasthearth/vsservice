package model

import "time"

// Season represents a competitive season of the Hunger Games game mode.
type Season struct {
	ID        string
	Number    int
	StartedAt time.Time
	EndedAt   *time.Time
}

func NewSeason(number int) *Season {
	return &Season{
		Number:    number,
		StartedAt: time.Now(),
	}
}

// End closes the season by recording the end timestamp.
func (s *Season) End() {
	now := time.Now()
	s.EndedAt = &now
}

// IsActive returns true when the season has not been closed yet.
func (s *Season) IsActive() bool {
	return s.EndedAt == nil
}
