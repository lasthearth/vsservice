package repository

import (
	"context"
	"time"

	questiondto "github.com/lasthearth/vsservice/internal/rules/internal/dto/mongo/question"
	"github.com/lasthearth/vsservice/internal/rules/model"
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
