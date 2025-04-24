package mongodto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongo"
	"github.com/lasthearth/vsservice/internal/rules/model"
	"github.com/samber/lo"
)

func VerificationFronModel(v *model.Verification) *Verification {
	answers := lo.Map(v.Answers, func(answer model.Answer, _ int) Answer {
		return *AnswerFromModel(&answer)
	})

	return &Verification{
		Model:   mongo.NewModel(),
		UserID:  v.UserID,
		Answers: answers,
	}
}

func AnswerFromModel(a *model.Answer) *Answer {
	return &Answer{
		Model:    mongo.NewModel(),
		Question: a.Question,
		Answer:   a.Answer,
	}
}
