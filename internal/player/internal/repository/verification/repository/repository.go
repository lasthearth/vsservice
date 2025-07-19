//go:generate goverter gen github.com/lasthearth/vsservice/internal/player/internal/repository/verification/repository
package repository

import (
	"context"
	"time"

	verificationdto "github.com/lasthearth/vsservice/internal/player/internal/dto/mongo/verification"
	"github.com/lasthearth/vsservice/internal/player/internal/model/verification"
	"github.com/lasthearth/vsservice/internal/player/internal/repository/verification/repository/repoerr"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

// goverter:converter
// goverter:output:file repomapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:ObjectIdToString
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTime
type Mapper interface {
	// goverter:ignore Model
	FromVerification(verification verification.Verification) verificationdto.Verification
	FromAnswer(answer verification.Answer) verificationdto.Answer
	FromAnswers([]verification.Answer) []verificationdto.Answer

	// goverter:autoMap Model
	ToVerification(dto verificationdto.Verification) verification.Verification
	ToVerifications([]verificationdto.Verification) []verification.Verification
	ToAnswer(dto verificationdto.Answer) verification.Answer
	ToAnswers([]verificationdto.Answer) []verification.Answer
}

func (r *Repository) Create(ctx context.Context, userId string, v verification.Verification) error {
	l := r.log.
		WithMethod("create").
		With(
			zap.String("user_id", userId),
			zap.Int("answers_count", len(v.Answers)),
		)

	l.Info("processing verification request")

	dto := r.mapper.FromVerification(v)

	l.Debug("inserting verification request into database")

	_, err := r.coll.InsertOne(ctx, dto)
	if err != nil {
		l.Error("failed to insert verification request", zap.Error(err))
		return err
	}

	l.Info("successfully created verification request")
	return nil
}

// GetVerificationStatusByUserGameName implements service.DbRepository
func (r *Repository) GetVerificationStatusByUserGameName(ctx context.Context, userGameName string) (verification.VerificationStatus, error) {
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

	var dto verificationdto.Verification
	if err := res.Decode(&dto); err != nil {
		r.log.Error("failed to decode verification request",
			zap.Error(err),
			zap.String("user_game_name", userGameName))
		return "", err
	}

	r.log.Debug("verification request exists",
		zap.String("user_game_name", userGameName))

	return verification.VerificationStatus(dto.Status), nil
}

// GetVerification implements service.VerificationDbRepository.
func (r *Repository) GetVerification(ctx context.Context, userId string) (*verification.Verification, error) {
	l := r.log.
		WithMethod("get_verification").
		With(zap.String("user_id", userId))

	l.Debug("retrieving verification request")

	res := r.coll.FindOne(ctx, bson.M{"user_id": userId})
	err := res.Err()
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, repoerr.ErrNotFound
		}

		l.Error("failed to find verification request", zap.Error(err))
		return nil, err
	}

	var verification verificationdto.Verification
	if err := res.Decode(&verification); err != nil {
		l.Error("failed to decode verification request", zap.Error(err))
		return nil, err
	}

	l.Debug("verification request retrieved")

	v := r.mapper.ToVerification(verification)

	return &v, nil
}

// GetVerificationRequests implements service.Repository.
func (r *Repository) GetVerificationRequests(ctx context.Context) ([]verification.Verification, error) {
	l := r.log.WithMethod("get_verification_requests")
	l.Info("retrieving all verification requests")

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	l.Debug("executing find query on verification collection")
	finded, err := r.coll.Find(ctx, bson.M{"status": verification.VerificationStatusPending})
	if err != nil {
		l.Error("find error", zap.Error(err))
		return nil, err
	}
	defer finded.Close(ctx)

	var verifications []verificationdto.Verification
	if err := finded.All(ctx, &verifications); err != nil {
		l.Error("cursor error", zap.Error(err))
		return nil, err
	}

	result := r.mapper.ToVerifications(verifications)

	l.Info("successfully retrieved verification requests", zap.Int("count", len(result)))
	return result, nil
}

// Update implements service.VerificationDbRepository.
func (r *Repository) Update(ctx context.Context, userId string, v verification.Verification) error {
	l := r.log.
		WithMethod("update").
		With(
			zap.String("user_id", userId),
			zap.Int("answers_count", len(v.Answers)),
		)

	l.Info("updating verification request in database")

	dto := r.mapper.FromVerification(v)

	// compute fields needed for update
	dtoBytes, _ := bson.Marshal(dto)
	var dtoMap bson.M
	bson.Unmarshal(dtoBytes, &dtoMap)
	delete(dtoMap, "_id")
	delete(dtoMap, "created_at")
	dtoMap["updated_at"] = time.Now()

	result, err := r.coll.UpdateOne(
		ctx,
		bson.M{"user_id": userId},
		bson.D{
			{Key: "$set", Value: dto},
		},
	)
	if err != nil {
		l.Error("failed to update verification request", zap.Error(err))
		return err
	}

	l.Info(
		"successfully updated verification request",
		zap.Int64("modified_count", result.ModifiedCount),
	)
	return nil
}
