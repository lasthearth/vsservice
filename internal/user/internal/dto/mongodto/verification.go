package mongodto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongo"
)

type Verification struct {
	mongo.Model `bson:",inline"`
	// User id from sso
	UserID  string
	Answers []Answer
}

type Answer struct {
	mongo.Model
	Question string
	Answer   string
}

func NewVerification(userID string, answers []Answer) *Verification {
	return &Verification{
		Model:   mongo.NewModel(),
		UserID:  userID,
		Answers: answers,
	}
}

func NewAnswer(question string, answer string) *Answer {
	return &Answer{
		Model:    mongo.NewModel(),
		Question: question,
		Answer:   answer,
	}
}
