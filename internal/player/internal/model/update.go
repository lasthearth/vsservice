package model

import "time"

type PlayerUpdate struct {
	UserId       *string
	UserName     *string
	UserGameName *string
	Avatar       *Avatar

	IsOnline *bool

	PreviousNickname      *string
	LastNicknameChangedAt *time.Time
}
