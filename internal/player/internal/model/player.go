package model

import (
	"time"

	"github.com/lasthearth/vsservice/internal/player/internal/model/stats"
	"github.com/lasthearth/vsservice/internal/player/internal/model/verification"
)

type Player struct {
	Id string
	// User id from sso
	UserId       string
	UserName     string
	UserGameName string

	// Nickname change tracking
	PreviousNickname      string
	LastNicknameChangedAt time.Time

	IsOnline bool

	Verification verification.Verification
	Stats        stats.Stats

	UpdatedAt time.Time
	CreatedAt time.Time
}

type PlayerUpdate struct {
	UserId       *string
	UserName     *string
	UserGameName *string

	IsOnline *bool

	PreviousNickname      *string
	LastNicknameChangedAt *time.Time
}
