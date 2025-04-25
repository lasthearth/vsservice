package repository

import (
	"context"

	"github.com/lasthearth/vsservice/internal/pkg/mongo"
	verificationdto "github.com/lasthearth/vsservice/internal/rules/dto/mongo/verification"
	"github.com/lasthearth/vsservice/internal/rules/model"
	"github.com/samber/lo"
)

func (r *Repository) VerificationRequest(ctx context.Context, userID, userGameName, contacts string, answers []model.Answer) error {
	dtoAnswers := lo.Map(answers, func(answer model.Answer, _ int) verificationdto.Answer {
		return *verificationdto.AnswerFromModel(&answer)
	})

	dto := verificationdto.Verification{
		Model:        mongo.NewModel(),
		UserID:       userID,
		UserGameName: userGameName,
		Contacts:     contacts,
		Answers:      dtoAnswers,
	}

	_, err := r.coll.InsertOne(ctx, dto)
	if err != nil {
		return err
	}

	return nil
}
