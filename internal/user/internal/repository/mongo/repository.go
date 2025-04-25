package repository

import (
	"context"

	"github.com/lasthearth/vsservice/internal/pkg/mongo"
	verificationdto "github.com/lasthearth/vsservice/internal/rules/dto/mongo/verification"
	"github.com/lasthearth/vsservice/internal/rules/model"
	"github.com/lasthearth/vsservice/internal/user/internal/service"
	"github.com/samber/lo"
)

func (r *Repository) VerificationRequest(ctx context.Context, opts service.VerifyOpts) error {
	dtoAnswers := lo.Map(opts.Answers, func(answer model.Answer, _ int) verificationdto.Answer {
		return *verificationdto.AnswerFromModel(&answer)
	})

	dto := verificationdto.Verification{
		Model:        mongo.NewModel(),
		UserID:       opts.UserID,
		UserName:     opts.UserName,
		UserGameName: opts.UserGameName,
		Contacts:     opts.Contacts,
		Answers:      dtoAnswers,
	}

	_, err := r.coll.InsertOne(ctx, dto)
	if err != nil {
		return err
	}

	return nil
}
