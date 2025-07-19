//go:generate goverter gen github.com/lasthearth/vsservice/internal/news/internal/repository
package repository

import (
	"context"
	"errors"

	"github.com/lasthearth/vsservice/internal/news/internal/dto"
	"github.com/lasthearth/vsservice/internal/news/internal/ierror"
	"github.com/lasthearth/vsservice/internal/news/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongo/pagination"
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
	// goverter:ignore Model
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

// DeleteNews implements service.Repository.
func (r *Repository) DeleteNews(ctx context.Context, id string) error {
	panic("unimplemented")
}
