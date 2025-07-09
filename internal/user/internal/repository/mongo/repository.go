package repository

import (
	"context"

	"github.com/lasthearth/vsservice/internal/user/internal/model"
	verificationdto "github.com/lasthearth/vsservice/internal/verification/dto/mongo/verification"
	verificationmodel "github.com/lasthearth/vsservice/internal/verification/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// GetVerificationStatusByUserGameName implements service.DbRepository
func (r *Repository) GetVerificationStatusByUserGameName(ctx context.Context, userGameName string) (verificationmodel.VerificationStatus, error) {
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

	return verificationmodel.VerificationStatus(verification.Status), nil
}

// GetVerificationStatus implements service.DbRepository
func (r *Repository) GetVerificationStatus(ctx context.Context, userID string) (verificationmodel.VerificationStatus, error) {
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

	return verificationmodel.VerificationStatus(verification.Status), nil
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

func (r *Repository) VerifyCode(ctx context.Context, userGameName string, code string) error {
	r.log.Debug("verifying code",
		zap.String("user_game_name", userGameName),
		zap.String("code", code))

	resp := r.coll.FindOneAndUpdate(ctx, bson.M{
		"user_game_name":    userGameName,
		"verification_code": code,
		"status":            verificationmodel.VerificationStatusApproved,
	}, bson.M{
		"$set": bson.M{
			"status": verificationmodel.VerificationStatusVerified,
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

const limit = 7

// SearchUsers implements service.DbRepository.
func (r *Repository) SearchUsers(ctx context.Context, query string) ([]model.User, error) {
	filter := bson.M{
		"$or": []bson.M{
			{"user_game_name": bson.M{"$regex": query, "$options": "i"}},
			{"user_name": bson.M{"$regex": query, "$options": "i"}},
		},
	}

	proj := options.Find().
		SetProjection(bson.D{
			{Key: "user_game_name", Value: 1},
			{Key: "user_name", Value: 1},
			{Key: "user_id", Value: 1},
		}).
		SetSort(bson.D{
			{Key: "_id", Value: 1},
		}).
		SetLimit(int64(limit))

	cursor, err := r.coll.Find(ctx, filter, proj)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var dtos []verificationdto.Verification
	if err := cursor.All(ctx, &dtos); err != nil {
		return nil, err
	}

	var users []model.User
	for _, dto := range dtos {
		user := model.User{
			UserId:   dto.UserID,
			UserName: dto.UserName,
			GameName: dto.UserGameName,
		}
		users = append(users, user)
	}

	return users, nil
}
