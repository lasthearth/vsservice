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

	// About carries the whole description. A pointer to an empty string means
	// "remove it": the repository unsets about and about_updated_at instead of
	// storing an empty string.
	About          *string
	AboutUpdatedAt *time.Time
}
