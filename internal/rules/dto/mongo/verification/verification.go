package verificationdto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongo"
)

type Verification struct {
	mongo.Model `bson:",inline"`
	// User id from sso
	UserID       string   `bson:"user_id"`
	UserName     string   `bson:"user_name"`
	UserGameName string   `bson:"user_game_name"`
	Contacts     string   `bson:"contacts"`
	Answers      []Answer `bson:"answers"`
}

type Answer struct {
	mongo.Model `bson:",inline"`
	Question    string `bson:"question"`
	Answer      string `bson:"answer"`
}
