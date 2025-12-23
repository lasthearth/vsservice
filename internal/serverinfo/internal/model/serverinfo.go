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
