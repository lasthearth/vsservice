package repository

import (
	"context"

	"github.com/lasthearth/vsservice/internal/rules/model"
	"github.com/lasthearth/vsservice/internal/user/internal/dto/mongodto"
	"github.com/samber/lo"
)

func (r *Repository) VerificationRequest(ctx context.Context, userID string, answers []model.Answer) error {
	dtoAnswers := lo.Map(answers, func(answer model.Answer, _ int) mongodto.Answer {
		return *mongodto.AnswerFromModel(&answer)
	})

	dto := mongodto.NewVerification(userID, dtoAnswers)

	_, err := r.coll.InsertOne(ctx, dto)
	if err != nil {
		return err
	}

	return nil
}
