//go:generate goverter gen github.com/lasthearth/vsservice/internal/news/internal/repository
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/lasthearth/vsservice/internal/news/internal/dto"
	"github.com/lasthearth/vsservice/internal/news/internal/ierror"
	"github.com/lasthearth/vsservice/internal/news/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/pkg/mongox/pagination"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

// goverter:converter
// goverter:output:file repomapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:ObjectIdToString
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTime
type Mapper interface {
	FromModels([]model.News) []dto.News
	// goverter:ignore Model ViewerIDs
	FromModel(model.News) dto.News

	ToModels(dto []dto.News) []model.News
	// goverter:autoMap Model
	// goverter:map Id Id
	ToModel(dto dto.News) model.News
}

// CreateNews implements service.Repository.
func (r *Repository) CreateNews(ctx context.Context, news *model.News) (*model.News, error) {
	l := r.logger.
		WithMethod("create").
		With(zap.String("title", news.Title))

	l.Info("creating news")

	ndto := r.mapper.FromModel(*news)
	ndto.Model = mongox.NewModel()
	ins, err := r.coll.InsertOne(ctx, ndto)
	if err != nil {
		return nil, err
	}

	finded := r.coll.FindOne(ctx, bson.M{"_id": ins.InsertedID})
	err = finded.Err()
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ierror.ErrNotFound
		}
		return nil, err
	}

	var created dto.News
	if err := finded.Decode(&created); err != nil {
		return nil, err
	}

	createdNews := r.mapper.ToModel(created)
	return &createdNews, nil
}

// ListNews implements service.Repository.
func (r *Repository) ListNews(
	ctx context.Context,
	next string,
	limit int,
) ([]model.News, string, error) {
	l := r.logger.
		WithMethod("list").
		With(
			zap.String("next", next),
			zap.Int("limit", limit),
		)

	l.Info("listing news")

	resp, err := pagination.Find[dto.News](
		ctx,
		r.coll,
		pagination.WithLimit(15),
		pagination.WithFilter(bson.M{"deleted_at": bson.M{"$exists": false}}),
	)
	if err != nil {
		l.Error("failed to list news", zap.Error(err))
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, "", ierror.ErrNewsNotFound
		}
		return nil, "", err
	}

	return r.mapper.ToModels(resp.Data), resp.Next, nil
}

// GetNewsById implements service.Repository.
func (r *Repository) GetNewsById(ctx context.Context, id string) (*model.News, error) {
	l := r.logger.
		WithMethod("get_by_id").
		With(zap.String("id", id))

	l.Info("getting news by id")

	objID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		l.Error("invalid object id", zap.Error(err))
		return nil, ierror.ErrNotFound
	}

	finded := r.coll.FindOne(ctx, bson.M{"_id": objID, "deleted_at": bson.M{"$exists": false}})
	err = finded.Err()
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			l.Warn("news not found", zap.Error(err))
			return nil, ierror.ErrNotFound
		}
		l.Error("failed to find news", zap.Error(err))
		return nil, err
	}

	var news dto.News
	if err := finded.Decode(&news); err != nil {
		l.Error("failed to decode news", zap.Error(err))
		return nil, err
	}

	newsModel := r.mapper.ToModel(news)

	return &newsModel, nil
}

// SoftDeleteNews implements service.Repository.
func (r *Repository) SoftDeleteNews(ctx context.Context, id string, deletedBy string) error {
	l := r.logger.
		WithMethod("soft_delete").
		With(zap.String("id", id), zap.String("deleted_by", deletedBy))

	l.Info("soft deleting news")

	objID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		l.Error("invalid object id", zap.Error(err))
		return ierror.ErrNotFound
	}

	now := time.Now()
	update := bson.M{"$set": bson.M{"deleted_at": now, "deleted_by": deletedBy}}

	result, err := r.coll.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		l.Error("failed to soft delete news", zap.Error(err))
		return err
	}

	if result.MatchedCount == 0 {
		l.Warn("news not found for soft deletion")
		return ierror.ErrNotFound
	}

	l.Info("news soft deleted successfully")
	return nil
}

// IncrementViewCount implements service.Repository.
func (r *Repository) IncrementViewCount(ctx context.Context, id string, userID string) error {
	l := r.logger.
		WithMethod("increment_view_count").
		With(zap.String("id", id), zap.String("user_id", userID))

	l.Info("incrementing view count")

	objID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		l.Error("invalid object id", zap.Error(err))
		return ierror.ErrNotFound
	}

	filter := bson.M{
		"_id":        objID,
		"viewer_ids": bson.M{"$ne": userID},
	}
	update := bson.M{
		"$addToSet": bson.M{"viewer_ids": userID},
		"$inc":      bson.M{"view_count": 1},
	}

	_, err = r.coll.UpdateOne(ctx, filter, update)
	if err != nil {
		l.Error("failed to increment view count", zap.Error(err))
		return err
	}

	l.Info("view count incremented successfully")
	return nil
}
