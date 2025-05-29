package model

import "time"

type Notification struct {
	Id        string
	UserId    string `validate:"required"`
	Title     string `validate:"required"`
	Message   string `validate:"required"`
	State     NotificationState
	CreatedAt time.Time
	UpdatedAt time.Time
}
