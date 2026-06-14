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
	Avatar       *Avatar

	// Nickname change tracking
	PreviousNickname      string
	LastNicknameChangedAt time.Time

	IsOnline bool

	Verification verification.Verification
	Stats        stats.Stats

	UpdatedAt time.Time
	CreatedAt time.Time
}

type Avatar struct {
	Original string
	X96      string
	X48      string
}

// SetVariant sets the avatar URL for a rendered size (px height).
func (a *Avatar) SetVariant(height int, path string) {
	switch height {
	case 96:
		a.X96 = path
	case 48:
		a.X48 = path
	}
}
