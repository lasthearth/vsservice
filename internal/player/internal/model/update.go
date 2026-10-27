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

	// ClearNicknameCooldown removes the recorded nickname change instead of
	// writing one: the repository unsets last_nickname_changed_at, which is how
	// a grant puts the player back inside their right to change the nickname.
	// It is the only way to write that field to "no change recorded", because a
	// zero time in LastNicknameChangedAt is indistinguishable from an unset one.
	ClearNicknameCooldown bool

	// About carries the whole description. A pointer to an empty string means
	// "remove it": the repository unsets about and about_updated_at instead of
	// storing an empty string.
	About          *string
	AboutUpdatedAt *time.Time
}
