package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	mongomodel "github.com/lasthearth/vsservice/internal/pkg/mongo"
	verificationdto "github.com/lasthearth/vsservice/internal/rules/dto/mongo/verification"
	"github.com/lasthearth/vsservice/internal/rules/model"
	"github.com/lasthearth/vsservice/internal/user/internal/service"
	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

func (r *Repository) CreateVerificationRequest(ctx context.Context, opts service.VerifyOpts) error {
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
		Model:            mongomodel.NewModel(),
		UserID:           opts.UserID,
		UserName:         opts.UserName,
		UserGameName:     opts.UserGameName,
		Contacts:         opts.Contacts,
		Answers:          dtoAnswers,
		VerificationCode: r.generateVerificationCode(),
		Status:           string(model.VerificationStatusPending),
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

// GetVerificationStatusByUserGameName implements service.DbRepository
func (r *Repository) GetVerificationStatusByUserGameName(ctx context.Context, userGameName string) (model.VerificationStatus, error) {
	r.log.Debug("checking if verification request exists",
		zap.String("user_game_name", userGameName))

	res := r.coll.FindOne(ctx, bson.M{"user_game_name": userGameName})
	if res.Err() != nil {
		if res.Err() == mongo.ErrNoDocuments {
			return "", nil
		}

		r.log.Error("failed to find verification request",
			zap.Error(res.Err()),
			zap.String("user_game_name", userGameName))
		return "", res.Err()
	}

	var verification verificationdto.Verification
	if err := res.Decode(&verification); err != nil {
		r.log.Error("failed to decode verification request",
			zap.Error(err),
			zap.String("user_game_name", userGameName))
		return "", err
	}

	r.log.Debug("verification request exists",
		zap.String("user_game_name", userGameName))

	return model.VerificationStatus(verification.Status), nil
}

// GetVerificationStatus implements service.DbRepository
func (r *Repository) GetVerificationStatus(ctx context.Context, userID string) (model.VerificationStatus, error) {
	r.log.Debug("checking if verification request exists",
		zap.String("user_id", userID))

	res := r.coll.FindOne(ctx, bson.M{"user_id": userID})
	if res.Err() != nil {
		if res.Err() == mongo.ErrNoDocuments {
			return "", nil
		}

		r.log.Error("failed to find verification request",
			zap.Error(res.Err()),
			zap.String("user_id", userID))
		return "", res.Err()
	}

	var verification verificationdto.Verification
	if err := res.Decode(&verification); err != nil {
		r.log.Error("failed to decode verification request",
			zap.Error(err),
			zap.String("user_id", userID))
		return "", err
	}

	r.log.Debug("verification request exists",
		zap.String("user_id", userID))

	return model.VerificationStatus(verification.Status), nil
}

func (r *Repository) GetVerificationCode(ctx context.Context, userID string) (string, error) {
	resp := r.coll.FindOne(ctx, bson.M{
		"user_id": userID,
	})

	if resp.Err() != nil {
		if resp.Err() == mongo.ErrNoDocuments {
			return "", ErrNotFound
		}

		r.log.Error("failed to find verification request",
			zap.Error(resp.Err()),
			zap.String("user_id", userID))
		return "", resp.Err()
	}
	var verification verificationdto.Verification
	if err := resp.Decode(&verification); err != nil {
		r.log.Error("decode error", zap.Error(err))
		return "", err
	}

	return verification.VerificationCode, nil
}

func (r *Repository) generateVerificationCode() string {
	b := make([]byte, 3)
	_, err := rand.Read(b)
	if err != nil {
		r.log.Error("failed to generate verification code",
			zap.Error(err))
		return ""
	}
	return hex.EncodeToString(b)
}

func (r *Repository) VerifyCode(ctx context.Context, userGameName string, code string) error {
	r.log.Debug("verifying code",
		zap.String("user_game_name", userGameName),
		zap.String("code", code))

	resp := r.coll.FindOneAndUpdate(ctx, bson.M{
		"user_game_name":    userGameName,
		"verification_code": code,
		"status":            model.VerificationStatusApproved,
	}, bson.M{
		"$set": bson.M{
			"status": model.VerificationStatusVerified,
		},
	})

	if resp.Err() != nil {
		if resp.Err() == mongo.ErrNoDocuments {
			return ErrNotFound
		}

		r.log.Error("failed to find verification request",
			zap.Error(resp.Err()),
			zap.String("user_game_name", userGameName))
		return resp.Err()
	}

	var verification verificationdto.Verification
	if err := resp.Decode(&verification); err != nil {
		r.log.Error("decode error", zap.Error(err))
		return err
	}

	return nil
}
