package dto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongo"
)

type User struct {
	mongo.Model `bson:",inline"`
	GameName    string `bson:"user_game_name"`
	UserName    string `bson:"user_name"`
	UserId      string `bson:"user_id"`
}
