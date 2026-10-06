package service

import (
	"context"
	"fmt"

	eventv1 "github.com/lasthearth/vsservice/gen/event/v1"
	"github.com/lasthearth/vsservice/internal/event/internal/model"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	defaultPageSize = 50
	maxPageSize     = 100
)

// CreateEvent implements eventv1.EventServiceServer.
func (s *Service) CreateEvent(ctx context.Context, req *eventv1.CreateEventRequest) (*eventv1.Event, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, ierror.Unauthenticated(err.Error())
	}

	if err := s.validateCover(req.GetCover()); err != nil {
		return nil, err
	}

	event, err := model.New(model.Details{
		Title:       req.GetTitle(),
		Description: req.GetDescription(),
		Cover:       req.GetCover(),
		Location:    req.GetLocation(),
		StartsAt:    timeOf(req.GetStartsAt()),
		EndsAt:      timePtr(req.GetEndsAt()),
	}, userID)
	if err != nil {
		return nil, err
	}

	created, err := s.repo.Create(ctx, event)
	if err != nil {
		return nil, err
	}

	// The calendar entry is the source of truth; a failed broadcast must not
	// undo it, so it is only logged.
	if err := s.cnuc.CreateNotification(
		ctx,
		"Новое событие",
		fmt.Sprintf("Событие: %s", created.Title),
		notificationuc.WithBroadcast(),
	); err != nil {
		s.logger.Error("failed to broadcast event notification", zap.String("event_id", created.Id), zap.Error(err))
	}

	return toProto(created), nil
}

// UpdateEvent implements eventv1.EventServiceServer.
func (s *Service) UpdateEvent(ctx context.Context, req *eventv1.UpdateEventRequest) (*eventv1.Event, error) {
	if err := s.validateCover(req.GetCover()); err != nil {
		return nil, err
	}

	event, err := s.repo.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	if err := event.Apply(model.Details{
		Title:       req.GetTitle(),
		Description: req.GetDescription(),
		Cover:       req.GetCover(),
		Location:    req.GetLocation(),
		StartsAt:    timeOf(req.GetStartsAt()),
		EndsAt:      timePtr(req.GetEndsAt()),
	}); err != nil {
		return nil, err
	}

	updated, err := s.repo.Update(ctx, event)
	if err != nil {
		return nil, err
	}

	return toProto(updated), nil
}

// DeleteEvent implements eventv1.EventServiceServer.
func (s *Service) DeleteEvent(ctx context.Context, req *eventv1.DeleteEventRequest) (*emptypb.Empty, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, ierror.Unauthenticated(err.Error())
	}

	if err := s.repo.SoftDelete(ctx, req.GetId(), userID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// GetEvent implements eventv1.EventServiceServer.
func (s *Service) GetEvent(ctx context.Context, req *eventv1.GetEventRequest) (*eventv1.Event, error) {
	event, err := s.repo.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	return toProto(event), nil
}

// ListEvents implements eventv1.EventServiceServer.
func (s *Service) ListEvents(ctx context.Context, req *eventv1.ListEventsRequest) (*eventv1.ListEventsResponse, error) {
	limit := int(req.GetPageSize())
	if limit <= 0 {
		limit = defaultPageSize
	}
	limit = min(limit, maxPageSize)

	now := s.now()
	list := s.repo.ListUpcoming
	if req.GetPast() {
		list = s.repo.ListPast
	}

	events, err := list(ctx, now, limit)
	if err != nil {
		return nil, err
	}

	return &eventv1.ListEventsResponse{Events: toProtos(events)}, nil
}

// validateCover accepts an empty cover or a URL from the media CDN.
func (s *Service) validateCover(cover string) error {
	if cover == "" {
		return nil
	}
	if err := s.mediaURL.Validate(cover); err != nil {
		return model.ErrInvalidCoverURL
	}
	return nil
}
