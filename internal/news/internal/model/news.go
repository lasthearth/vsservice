package model

import "time"

type News struct {
	Id        string
	Title     string `validate:"required"`
	Preview   string
	Content   string `validate:"required"`
	CreatedAt time.Time
	ViewCount int64
}
