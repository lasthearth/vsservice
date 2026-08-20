package service

import (
	"context"

	newsv1 "github.com/lasthearth/vsservice/gen/news/v1"
	"github.com/lasthearth/vsservice/internal/news/internal/model"
	"github.com/lasthearth/vsservice/internal/news/internal/repository"
)

//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/news/internal/service

// goverter:converter
// goverter:output:file sermapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTimestamp
type Mapper interface {
	// goverter:ignore state sizeCache unknownFields
	ToProto(model.News) *newsv1.News
	ToProtos([]model.News) []*newsv1.News
}

var _ Repository = (*repository.Repository)(nil)

type Repository interface {
	CreateNews(ctx context.Context, news *model.News) (*model.News, error)
	ListNews(ctx context.Context, next string, limit int) ([]model.News, string, error)
	GetNewsById(ctx context.Context, id string) (*model.News, error)
	SoftDeleteNews(ctx context.Context, id string, deletedBy string) error
	IncrementViewCount(ctx context.Context, id string, userID string) error
	GetNewsViewCount(ctx context.Context, id string) (int64, error)
}
