package repository

import (
	"context"

	"github.com/lasthearth/vsservice/internal/pkg/mongo"
	verificationdto "github.com/lasthearth/vsservice/internal/rules/dto/mongo/verification"
	"github.com/lasthearth/vsservice/internal/rules/model"
	"github.com/lasthearth/vsservice/internal/user/internal/service"
	"github.com/samber/lo"
	"go.uber.org/zap"
)

func (r *Repository) VerificationRequest(ctx context.Context, opts service.VerifyOpts) error {
	r.log.Info("processing verification request",
		zap.String("user_id", opts.UserID),
		zap.String("user_name", opts.UserName),
		zap.String("user_game_name", opts.UserGameName),
		zap.Int("answers_count", len(opts.Answers)))

	r.log.Debug("mapping answers to DTO format",
		zap.Int("answers_count", len(opts.Answers)))

	dtoAnswers := lo.Map(opts.Answers, func(answer model.Answer, _ int) verificationdto.Answer {
		return *verificationdto.AnswerFromModel(&answer)
	})

	r.log.Debug("successfully mapped answers to DTO format")

	dto := verificationdto.Verification{
		Model:        mongo.NewModel(),
		UserID:       opts.UserID,
		UserName:     opts.UserName,
		UserGameName: opts.UserGameName,
		Contacts:     opts.Contacts,
		Answers:      dtoAnswers,
	}

	r.log.Debug("inserting verification request into database",
		zap.String("user_id", opts.UserID),
		zap.String("model_id", dto.ID.Hex()))

	_, err := r.coll.InsertOne(ctx, dto)
	if err != nil {
		r.log.Error("failed to insert verification request",
			zap.Error(err),
			zap.String("user_id", opts.UserID),
			zap.String("model_id", dto.ID.Hex()))
		return err
	}

	r.log.Info("successfully created verification request",
		zap.String("user_id", opts.UserID),
		zap.String("model_id", dto.ID.Hex()))
	return nil
}
