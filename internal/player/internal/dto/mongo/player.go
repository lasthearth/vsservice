package dto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	verificationdto "github.com/lasthearth/vsservice/internal/player/internal/dto/mongo/verification"
	"github.com/lasthearth/vsservice/internal/player/internal/model/stats"
)

type Player struct {
	mongox.Model `bson:",inline"`
	UserId       string `bson:"user_id"`
	UserName     string `bson:"user_name"`
	UserGameName string `bson:"user_game_name"`

	// Nickname change tracking
	PreviousNickname      string    `bson:"previous_nickname"`
	LastNicknameChangedAt time.Time `bson:"last_nickname_changed_at"`

	Verification verificationdto.Verification `bson:",inline"`
	Stats        stats.Stats                  `bson:",inline"`
}
