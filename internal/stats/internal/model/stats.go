package model

import "time"

type Stats struct {
	ID            string
	Name          string
	DeathCount    int
	Seeds         []int
	HoursPlayed   float32
	LastOnline    time.Time
	PlayersKilled int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
