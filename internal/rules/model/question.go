package model

import "time"

type Question struct {
	ID        string
	Question  string
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}
