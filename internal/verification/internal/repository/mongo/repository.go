package repository

import (
	"context"
	"time"

	mongomodel "github.com/lasthearth/vsservice/internal/pkg/mongo"
	verificationdto "github.com/lasthearth/vsservice/internal/verification/dto/mongo/verification"
	"github.com/lasthearth/vsservice/internal/verification/internal/pkg/code"
	"github.com/lasthearth/vsservice/internal/verification/internal/repository/mongo/repoerr"
	"github.com/lasthearth/vsservice/internal/verification/internal/service"
	"github.com/lasthearth/vsservice/internal/verification/model"
	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

func (r *Repository) Create(ctx context.Context, opts service.VerifyOpts) error {
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
		VerificationCode: code.Generate(),
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

// GetVerification implements service.VerificationDbRepository.
func (r *Repository) GetVerification(ctx context.Context, userID string) (*model.Verification, error) {
	r.log.Debug("retrieving verification request",
		zap.String("user_id", userID))

	res := r.coll.FindOne(ctx, bson.M{"user_id": userID})
	if res.Err() != nil {
		if res.Err() == mongo.ErrNoDocuments {
			return nil, repoerr.ErrNotFound
		}

		r.log.Error("failed to find verification request",
			zap.Error(res.Err()),
			zap.String("user_id", userID))
		return nil, res.Err()
	}

	var verification verificationdto.Verification
	if err := res.Decode(&verification); err != nil {
		r.log.Error("failed to decode verification request",
			zap.Error(err),
			zap.String("user_id", userID))
		return nil, err
	}

	r.log.Debug("verification request retrieved",
		zap.String("user_id", userID))

	return verification.ToModel(), nil
}

// GetVerificationRequests implements service.Repository.
func (r *Repository) GetVerificationRequests(ctx context.Context) ([]*model.Verification, error) {
	r.log.Info("retrieving all verification requests")

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	r.log.Debug("executing find query on verification collection")
	finded, err := r.coll.Find(ctx, bson.M{"status": model.VerificationStatusPending})
	if err != nil {
		r.log.Error("find error", zap.Error(err))
		return nil, err
	}
	defer finded.Close(ctx)

	var verifications []verificationdto.Verification
	if err := finded.All(ctx, &verifications); err != nil {
		r.log.Error("cursor error", zap.Error(err))
		return nil, err
	}

	result := lo.Map(verifications, func(item verificationdto.Verification, index int) *model.Verification {
		return item.ToModel()
	})

	r.log.Info("successfully retrieved verification requests", zap.Int("count", len(result)))
	return result, nil
}

func (r *Repository) Approve(ctx context.Context, userId string) error {
	r.log.Info("approving verification request", zap.String("user_id", userId))

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	r.log.Debug("executing update query",
		zap.String("user_id", userId),
		zap.String("collection", "verifications"))

	result, err := r.coll.UpdateOne(ctx, bson.M{"user_id": userId},
		bson.D{
			{
				Key: "$set",
				Value: bson.D{
					{Key: "status", Value: model.VerificationStatusApproved},
					{Key: "updated_at", Value: time.Now()},
				},
			},
		})
	if err != nil {
		r.log.Error("update error", zap.Error(err), zap.String("user_id", userId))
		return err
	}

	r.log.Info("successfully approved verification request",
		zap.String("user_id", userId),
		zap.Int64("updated_count", result.ModifiedCount))
	return nil
}

// Reject implements service.VerificationDbRepository.
func (r *Repository) Reject(ctx context.Context, userId, rejectionReason string) error {
	r.log.Info("rejecting verification request", zap.String("user_id", userId))

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	r.log.Debug("executing update query",
		zap.String("user_id", userId),
		zap.String("collection", "verifications"))

	result, err := r.coll.UpdateOne(ctx, bson.M{"user_id": userId},
		bson.D{
			{
				Key: "$set",
				Value: bson.D{
					{Key: "status", Value: model.VerificationStatusRejected},
					{Key: "rejection_reason", Value: rejectionReason},
					{Key: "updated_at", Value: time.Now()},
				},
			},
		})
	if err != nil {
		r.log.Error("update error", zap.Error(err), zap.String("user_id", userId))
		return err
	}

	r.log.Info("successfully rejected verification request",
		zap.String("user_id", userId),
		zap.Int64("updated_count", result.ModifiedCount))
	return nil
}

// Update implements service.VerificationDbRepository.
func (r *Repository) Update(ctx context.Context, opts service.VerifyOpts) error {
	r.log.Info("updating verification request",
		zap.String("user_id", opts.UserID),
		zap.String("user_name", opts.UserName),
		zap.String("user_game_name", opts.UserGameName),
		zap.Int("answers_count", len(opts.Answers)))

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	r.log.Debug("mapping answers to DTO format",
		zap.Int("answers_count", len(opts.Answers)))

	dtoAnswers := lo.Map(opts.Answers, func(answer model.Answer, _ int) verificationdto.Answer {
		return *verificationdto.AnswerFromModel(&answer)
	})

	r.log.Debug("successfully mapped answers to DTO format")

	updateFields := bson.D{
		{Key: "user_name", Value: opts.UserName},
		{Key: "user_game_name", Value: opts.UserGameName},
		{Key: "contacts", Value: opts.Contacts},
		{Key: "answers", Value: dtoAnswers},
		{Key: "status", Value: string(model.VerificationStatusPending)},
		{Key: "rejection_reason", Value: ""},
		{Key: "updated_at", Value: time.Now()},
	}

	r.log.Debug("updating verification request in database",
		zap.String("user_id", opts.UserID))

	result, err := r.coll.UpdateOne(ctx, bson.M{"user_id": opts.UserID},
		bson.D{
			{Key: "$set", Value: updateFields},
		})
	if err != nil {
		r.log.Error("failed to update verification request",
			zap.Error(err),
			zap.String("user_id", opts.UserID))
		return err
	}

	r.log.Info("successfully updated verification request",
		zap.String("user_id", opts.UserID),
		zap.Int64("modified_count", result.ModifiedCount))
	return nil
}
