package model

import "time"

type Entry struct {
	UserId      string
	Name        string
	TotalHours  float64
	TotalDeaths int
	TotalKills  int
	// LastOnline is the latest last-online mark across the player's stats
	// documents; nil when the game has not recorded one.
	LastOnline *time.Time
}
