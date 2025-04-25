package repository

import (
	"context"
	"time"

	verificationdto "github.com/lasthearth/vsservice/internal/rules/dto/mongo/verification"
	questiondto "github.com/lasthearth/vsservice/internal/rules/internal/dto/mongo/question"
	"github.com/lasthearth/vsservice/internal/rules/model"
	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"
)

func (r *Repository) CreateQuestion(ctx context.Context, question *model.Question) error {
	_, err := r.questionColl.InsertOne(ctx, questiondto.FromModel(question))
	if err != nil {
		return err
	}
	return nil
}

func (r *Repository) GetRandomQuestions(ctx context.Context, count int) ([]*model.Question, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pipeline := bson.A{
		bson.D{{
			Key: "$sample",
			Value: bson.D{{
				Key:   "size",
				Value: count,
			}},
		}},
	}
	cur, err := r.questionColl.Aggregate(ctx, pipeline)
	if err != nil {
		r.log.Error("aggregate error", zap.Error(err))
		return nil, err
	}
	defer cur.Close(ctx)

	var questions []questiondto.Question
	if err := cur.All(ctx, &questions); err != nil {
		r.log.Error("cursor error", zap.Error(err))
		return nil, err
	}

	result := lo.Map(questions, func(item questiondto.Question, index int) *model.Question {
		return item.ToModel()
	})

	return result, nil
}

// GetVerificationRequests implements service.Repository.
func (r *Repository) GetVerificationRequests(ctx context.Context) ([]*model.Verification, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	finded, err := r.verificationColl.Find(ctx, bson.D{})
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

	return result, nil
}

func (r *Repository) DeleteVerificationRequest(ctx context.Context, userId string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if _, err := r.verificationColl.DeleteMany(ctx, bson.D{{Key: "user_id", Value: userId}}); err != nil {
		r.log.Error("delete error", zap.Error(err))
		return err
	}

	return nil
}
