package verificationdto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongo"
	"github.com/lasthearth/vsservice/internal/rules/model"
	"github.com/samber/lo"
)

func FromModel(v *model.Verification) *Verification {
	answers := lo.Map(v.Answers, func(answer model.Answer, _ int) Answer {
		return *AnswerFromModel(&answer)
	})

	return &Verification{
		Model:        mongo.NewModel(),
		UserID:       v.UserID,
		UserName:     v.UserName,
		UserGameName: v.UserGameName,
		Contacts:     v.Contacts,
		Status:       string(v.Status),
		Answers:      answers,
	}
}

func AnswerFromModel(a *model.Answer) *Answer {
	return &Answer{
		Model:    mongo.NewModel(),
		Question: a.Question,
		Answer:   a.Answer,
	}
}

func (v *Verification) ToModel() *model.Verification {
	answers := lo.Map(v.Answers, func(answer Answer, _ int) model.Answer {
		return *AnswerToModel(&answer)
	})

	return &model.Verification{
		ID:           v.ID.Hex(),
		UserID:       v.UserID,
		UserName:     v.UserName,
		UserGameName: v.UserGameName,
		Answers:      answers,
		Contacts:     v.Contacts,
		Status:       model.VerificationStatus(v.Status),
		UpdatedAt:    v.UpdatedAt,
		CreatedAt:    v.CreatedAt,
	}
}

func AnswerToModel(a *Answer) *model.Answer {
	return &model.Answer{
		ID:        a.ID.Hex(),
		Question:  a.Question,
		Answer:    a.Answer,
		CreatedAt: a.CreatedAt,
	}
}
