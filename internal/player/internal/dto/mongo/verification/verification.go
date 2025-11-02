package verificationdto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
)

type Verification struct {
	mongox.Model `bson:",inline"`
	// User id from sso
	UserId           string   `bson:"user_id"`
	UserName         string   `bson:"user_name"`
	UserGameName     string   `bson:"user_game_name"`
	Answers          []Answer `bson:"answers"`
	Contacts         string   `bson:"contacts"`
	Status           string   `bson:"status"`
	VerificationCode string   `bson:"verification_code"`
	RejectionReason  string   `bson:"rejection_reason"`
}
