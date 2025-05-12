package model

import "time"

type Question struct {
	ID        string
	Question  string
	CreatedAt time.Time
	UpdatedAt time.Time
}
