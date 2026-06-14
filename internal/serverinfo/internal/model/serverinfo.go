package model

import "time"

type ServerInfo struct {
	Id          string
	WorldTime   string
	TotalOnline int
	MaxOnline   int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// SetWorldTime sets the in-game world time.
func (s *ServerInfo) SetWorldTime(t string) { s.WorldTime = t }

// SetTotalOnline sets the current online player count.
func (s *ServerInfo) SetTotalOnline(n int) { s.TotalOnline = n }
