package dto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongo"
	verificationdto "github.com/lasthearth/vsservice/internal/player/internal/dto/mongo/verification"
	"github.com/lasthearth/vsservice/internal/player/internal/model/stats"
)

type Player struct {
	mongo.Model  `bson:",inline"`
	UserId       string `bson:"user_id"`
	UserName     string `bson:"user_name"`
	UserGameName string `bson:"user_game_name"`

	Verification verificationdto.Verification `bson:"verification"`
	Stats        stats.Stats                  `bson:"stats"`
}
