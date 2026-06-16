package service

import (
	"context"

	newsv1 "github.com/lasthearth/vsservice/gen/news/v1"
	"github.com/lasthearth/vsservice/internal/news/internal/model"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// CreateNews implements newsv1.NewsServiceServer.
func (s *Service) CreateNews(ctx context.Context, req *newsv1.CreateNewsRequest) (*newsv1.News, error) {
	if err := s.mediaUrl.Validate(req.GetPreview()); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid preview url")
	}

	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	news := &model.News{
		Title:     req.GetTitle(),
		Content:   req.GetContent(),
		Preview:   req.GetPreview(),
		CreatedBy: userID,
	}

	if err := s.validator.Struct(news); err != nil {
		return nil, err
	}

	created, err := s.repo.CreateNews(ctx, news)
	if err != nil {
		return nil, err
	}

	if err := s.cnuc.CreateNotification(
		ctx,
		"Новая новость",
		"Новость: "+req.GetTitle(),
		notificationuc.WithBroadcast(),
	); err != nil {
		return nil, err
	}

	return s.mapper.ToProto(*created), nil
}

// ListNews implements newsv1.NewsServiceServer.
func (s *Service) ListNews(ctx context.Context, req *newsv1.ListNewsRequest) (*newsv1.ListNewsResponse, error) {
	limit := min(int(req.GetPageSize()), 50)
	if req.GetPageSize() == 0 {
		limit = 15
	}
	news, next, err := s.repo.ListNews(ctx, req.GetPageToken(), limit)
	if err != nil {
		return nil, err
	}

	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	s.logger.Info("userid news inc", zap.String("user_id", userID))
	for _, v := range news {
		s.logger.Info("inc news")
		if err := s.repo.IncrementViewCount(ctx, v.Id, userID); err != nil {
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
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	_, err = s.repo.GetNewsById(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	err = s.repo.SoftDeleteNews(ctx, req.GetId(), userID)
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// GetNews implements newsv1.NewsServiceServer.
func (s *Service) GetNews(ctx context.Context, req *newsv1.GetNewsRequest) (*newsv1.News, error) {
	if userID, err := interceptor.GetUserID(ctx); err == nil {
		if err := s.repo.IncrementViewCount(ctx, req.GetId(), userID); err != nil {
			s.logger.Error("failed to increment view count", zap.Error(err))
		}
	}

	news, err := s.repo.GetNewsById(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	return s.mapper.ToProto(*news), nil
}
