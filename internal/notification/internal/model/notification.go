package model

import "time"

type Notification struct {
	ID        string
	UserID    string `validate:"required"`
	Title     string `validate:"required"`
	Message   string `validate:"required"`
	State     NotificationState
	CreatedAt time.Time
	UpdatedAt time.Time
}
