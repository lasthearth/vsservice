package service

import (
	"bytes"
	"context"
	"fmt"
	"mime"

	"github.com/google/uuid"
	newsv1 "github.com/lasthearth/vsservice/gen/news/v1"
	"github.com/lasthearth/vsservice/internal/news/internal/model"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

// CreateNews implements newsv1.NewsServiceServer.
func (s *Service) CreateNews(ctx context.Context, req *newsv1.CreateNewsRequest) (*newsv1.News, error) {
	bucketName := "news"
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	filename := id.String()
	fileExt := ".webp"

	rd := bytes.NewReader(req.Preview)

	path := fmt.Sprintf("%s%s", filename, fileExt)
	ct := mime.TypeByExtension(fileExt)

	_, err = s.storage.UploadObject(
		ctx,
		bucketName,
		path,
		rd,
		int64(len(req.Preview)),
		ct,
	)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s/%s", s.config.CdnUrl, bucketName, path)
	news := &model.News{
		Title:   req.Title,
		Content: req.Content,
		Preview: url,
	}

	err = s.validator.Struct(news)
	if err != nil {
		return nil, err
	}

	created, err := s.repo.CreateNews(ctx, news)
	if err != nil {
		return nil, err
	}

	if err := s.cnuc.CreateNotification(
		ctx,
		"Новая новость",
		fmt.Sprintf("Новость: %s", req.Title),
		notificationuc.WithBroadcast(),
	); err != nil {
		return nil, err
	}

	return s.mapper.ToProto(*created), nil
}

// ListNews implements newsv1.NewsServiceServer.
func (s *Service) ListNews(ctx context.Context, req *newsv1.ListNewsRequest) (*newsv1.ListNewsResponse, error) {
	limit := min(int(req.PageSize), 50)
	if req.PageSize == 0 {
		limit = 15
	}
	news, next, err := s.repo.ListNews(ctx, req.PageToken, limit)
	if err != nil {
		return nil, err
	}

	for _, v := range news {
		if err := s.repo.IncrementViewCount(ctx, v.Id); err != nil {
			s.logger.Error("failed to increment view count", zap.Error(err))
		}
	}

	return &newsv1.ListNewsResponse{
		News:          s.mapper.ToProtos(news),
		NextPageToken: next,
	}, nil
}

// DeleteNews implements newsv1.NewsServiceServer.
func (s *Service) DeleteNews(ctx context.Context, req *newsv1.DeleteNewsRequest) (*emptypb.Empty, error) {
	_, err := s.repo.GetNewsById(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	err = s.repo.DeleteNews(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// GetNews implements newsv1.NewsServiceServer.
func (s *Service) GetNews(ctx context.Context, req *newsv1.GetNewsRequest) (*newsv1.News, error) {
	if err := s.repo.IncrementViewCount(ctx, req.Id); err != nil {
		s.logger.Error("failed to increment view count", zap.Error(err))
	}

	news, err := s.repo.GetNewsById(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return s.mapper.ToProto(*news), nil
}
