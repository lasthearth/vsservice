package repository

import (
	"context"
	"time"

	verificationdto "github.com/lasthearth/vsservice/internal/rules/dto/mongo/verification"
	"github.com/lasthearth/vsservice/internal/rules/model"
	questiondto "github.com/lasthearth/vsservice/internal/verification/internal/dto/mongo/question"
	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"
)

func (r *Repository) CreateQuestion(ctx context.Context, question *model.Question) error {
	r.log.Info("creating new question",
		zap.String("question_id", question.ID),
		zap.String("text", question.Question))

	_, err := r.questionColl.InsertOne(ctx, questiondto.FromModel(question))
	if err != nil {
		r.log.Error("failed to insert question",
			zap.Error(err),
			zap.String("question_id", question.ID))
		return err
	}

	r.log.Info("successfully created question", zap.String("question_id", question.ID))
	return nil
}

func (r *Repository) GetRandomQuestions(ctx context.Context, count int) ([]*model.Question, error) {
	r.log.Info("getting random questions", zap.Int("count", count))

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

	r.log.Debug("executing aggregation query",
		zap.Any("pipeline", pipeline),
		zap.Int("requested_count", count))

	cur, err := r.questionColl.Aggregate(ctx, pipeline)
	if err != nil {
		r.log.Error("aggregate error", zap.Error(err), zap.Int("requested_count", count))
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

	r.log.Info("successfully retrieved random questions",
		zap.Int("requested_count", count),
		zap.Int("retrieved_count", len(result)))
	return result, nil
}

// GetVerificationRequests implements service.Repository.
func (r *Repository) GetVerificationRequests(ctx context.Context) ([]*model.Verification, error) {
	r.log.Info("retrieving all verification requests")

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	r.log.Debug("executing find query on verification collection")
	finded, err := r.verificationColl.Find(ctx, bson.M{"status": model.VerificationStatusPending})
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

func (r *Repository) ApproveVerificationRequest(ctx context.Context, userId string) error {
	r.log.Info("approving verification request", zap.String("user_id", userId))

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	r.log.Debug("executing update query",
		zap.String("user_id", userId),
		zap.String("collection", "verifications"))

	result, err := r.verificationColl.UpdateOne(ctx, bson.M{"user_id": userId},
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
