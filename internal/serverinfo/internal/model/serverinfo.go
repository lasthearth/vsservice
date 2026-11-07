package model

import "time"

// ServerInfo is the singleton document the game server writes and this service
// only reads. It carries no setters: nothing in this domain is allowed to change
// the world time or the online count, and the sweeper's zeroing of a stale
// total_online is a filtered bulk write, not a model transition.
type ServerInfo struct {
	Id          string
	WorldTime   string
	TotalOnline int
	MaxOnline   int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
