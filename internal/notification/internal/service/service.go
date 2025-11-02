package service

import (
	"context"
	"errors"
	"strings"

	notificationv1 "github.com/lasthearth/vsservice/gen/notification/v1"
	"github.com/lasthearth/vsservice/internal/notification/internal/model"
	"github.com/lasthearth/vsservice/internal/notification/internal/repository/repoerr"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// goverter:converter
// goverter:output:file sermapper/notification.go
// goverter:extend StateToProto
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTimestamp
type NotificationMapper interface {
	// goverter:ignore state sizeCache unknownFields
	ToProto(model.Notification) *notificationv1.Notification
	ToProtos([]model.Notification) []*notificationv1.Notification
}

type Repository interface {
	ListNotifications(ctx context.Context, limit int, userId, pageToken, orderBy string) ([]model.Notification, string, error)
	MarkNotificationRead(ctx context.Context, id string) error
}

// ListNotifications implements notificationv1.NotificationServiceServer.
func (s *Service) ListNotifications(ctx context.Context, req *notificationv1.ListNotificationsRequest) (*notificationv1.ListNotificationsResponse, error) {
	l := s.log.WithMethod("list_notifications")
	limit := min(int(req.PageSize), 15)

	pageToken := strings.TrimSpace(req.PageToken)
	orderBy := strings.ToLower(strings.TrimSpace(req.OrderBy))

	_, err := mongox.ParseObjectID(pageToken)
	if err != nil {
		l.Debug("failed to parse page token, make it empty", zap.Error(err))
		pageToken = ""
	}

	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		l.Error("failed to get user id", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get user id")
	}

	notifications, next, err := s.repo.ListNotifications(
		ctx,
		limit,
		uid,
		pageToken,
		orderBy,
	)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotificationsNotFound) {
			return nil, status.Error(codes.NotFound, "notifications not found")
		}
		l.Error("failed to list notifications", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list notifications")
	}

	return &notificationv1.ListNotificationsResponse{
		Notifications: s.mapper.ToProtos(notifications),
		NextPageToken: next,
	}, nil
}

// MarkAsRead implements notificationv1.NotificationServiceServer.
func (s *Service) MarkAsRead(ctx context.Context, req *notificationv1.MarkAsReadRequest) (*emptypb.Empty, error) {
	l := s.log.WithMethod("mark_as_read")

	_, err := mongox.ParseObjectID(req.Id)
	if err != nil {
		l.Error("failed to parse notification id", zap.Error(err))
		return nil, status.Error(codes.InvalidArgument, "invalid notification id")
	}

	if err := s.repo.MarkNotificationRead(ctx, req.Id); err != nil {
		return nil, status.Error(codes.Internal, "failed to mark notification as read")
	}

	return &emptypb.Empty{}, nil
}
