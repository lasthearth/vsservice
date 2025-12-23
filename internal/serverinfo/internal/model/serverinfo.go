package model

import "time"

type ServerInfo struct {
	Id          string
	WorldTime   string
	TotalOnline int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
