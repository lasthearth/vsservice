package dto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	verificationdto "github.com/lasthearth/vsservice/internal/player/internal/dto/mongo/verification"
	"github.com/lasthearth/vsservice/internal/player/internal/model/stats"
)

type Player struct {
	mongox.Model `bson:",inline"`
	UserId       string  `bson:"user_id"`
	UserName     string  `bson:"user_name"`
	UserGameName string  `bson:"user_game_name"`
	Avatar       *Avatar `bson:"avatar"`

	IsOnline bool `bson:"is_online"`

	// About is the description the player wrote about themselves. Absent when
	// there is none or a moderator hid it, which is why it is omitempty: the
	// repository unsets both fields instead of storing an empty string.
	About          string     `bson:"about,omitempty"`
	AboutUpdatedAt *time.Time `bson:"about_updated_at,omitempty"`

	// Nickname change tracking
	PreviousNickname      string    `bson:"previous_nickname"`
	LastNicknameChangedAt time.Time `bson:"last_nickname_changed_at"`

	Verification verificationdto.Verification `bson:",inline"`
	Stats        stats.Stats                  `bson:",inline"`
}

type Avatar struct {
	Original string `bson:"original"`
	X96      string `bson:"x96"`
	X48      string `bson:"x48"`
}
