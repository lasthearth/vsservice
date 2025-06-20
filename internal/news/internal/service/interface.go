package service

import (
	"context"
	"io"

	newsv1 "github.com/lasthearth/vsservice/gen/news/v1"
	"github.com/lasthearth/vsservice/internal/news/internal/model"
	"github.com/lasthearth/vsservice/internal/news/internal/repository"
	"github.com/lasthearth/vsservice/internal/pkg/storage"
	"github.com/minio/minio-go/v7"
)

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
	DeleteNews(ctx context.Context, id string) error
}

var _ Storage = (*storage.Storage)(nil)

type Storage interface {
	BucketExists(ctx context.Context, bucketName string) (bool, error)
	MakeBucketPublic(ctx context.Context, bucketName string) error
	CreateBucket(ctx context.Context, bucketName string) error
	UploadObject(
		ctx context.Context,
		bucketName, objectName string,
		reader io.Reader,
		size int64,
		contentType string,
	) (*minio.UploadInfo, error)
}
