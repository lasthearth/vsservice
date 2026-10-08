package service

import (
	"context"
	"time"

	eventv1 "github.com/lasthearth/vsservice/gen/event/v1"
	"github.com/lasthearth/vsservice/internal/event/internal/model"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
)

const (
	defaultAttendeesPageSize = 100
	maxAttendeesPageSize     = 200
)

// moscow is the server's time zone, used in notification texts.
var moscow = time.FixedZone("MSK", 3*60*60)

// SetAttendance implements eventv1.EventServiceServer.
func (s *Service) SetAttendance(ctx context.Context, req *eventv1.SetAttendanceRequest) (*eventv1.SetAttendanceResponse, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, ierror.Unauthenticated(err.Error())
	}

	event, err := s.repo.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	now := s.now()
	if err := event.AcceptsAttendance(now); err != nil {
		return nil, err
	}

	if err := s.repo.SetAttendance(ctx, event.Id, userID, req.GetAttending(), now); err != nil {
		return nil, err
	}

	summary, err := s.repo.Attendees(ctx, []string{event.Id})
	if err != nil {
		return nil, err
	}
	a := summary[event.Id]

	return &eventv1.SetAttendanceResponse{
		Attending:       req.GetAttending(),
		AttendeeCount:   a.Count,
		AttendeePreview: a.Preview,
	}, nil
}

// ListAttendees implements eventv1.EventServiceServer.
func (s *Service) ListAttendees(ctx context.Context, req *eventv1.ListAttendeesRequest) (*eventv1.ListAttendeesResponse, error) {
	event, err := s.repo.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	limit := int(req.GetPageSize())
	if limit <= 0 {
		limit = defaultAttendeesPageSize
	}
	limit = min(limit, maxAttendeesPageSize)

	ids, total, err := s.repo.ListAttendees(ctx, event.Id, limit)
	if err != nil {
		return nil, err
	}

	return &eventv1.ListAttendeesResponse{UserIds: ids, Total: int32(total)}, nil
}

// ListMyEvents implements eventv1.EventServiceServer.
func (s *Service) ListMyEvents(ctx context.Context, req *eventv1.ListMyEventsRequest) (*eventv1.ListEventsResponse, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, ierror.Unauthenticated(err.Error())
	}

	limit := int(req.GetPageSize())
	if limit <= 0 {
		limit = defaultPageSize
	}
	limit = min(limit, maxPageSize)

	events, err := s.repo.ListMine(ctx, userID, s.now(), req.GetPast(), limit)
	if err != nil {
		return nil, err
	}

	return s.listWithAttendees(ctx, events)
}

// withAttendees maps one event with its sign-up summary.
func (s *Service) withAttendees(ctx context.Context, event model.Event) (*eventv1.Event, error) {
	list, err := s.listWithAttendees(ctx, []model.Event{event})
	if err != nil {
		return nil, err
	}
	return list.GetEvents()[0], nil
}

// listWithAttendees maps events with their sign-up summaries, counted in one
// aggregation for the whole page.
func (s *Service) listWithAttendees(ctx context.Context, events []model.Event) (*eventv1.ListEventsResponse, error) {
	ids := make([]string, 0, len(events))
	for _, e := range events {
		ids = append(ids, e.Id)
	}

	summaries, err := s.repo.Attendees(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := s.mapper.ToProtos(events)
	for i, e := range events {
		a := summaries[e.Id]
		out[i].AttendeeCount = a.Count
		out[i].AttendeePreview = a.Preview
	}
	return &eventv1.ListEventsResponse{Events: out}, nil
}

// notifyAttendees sends one notification to every attendee. The change that
// caused it already stands, so failures are only logged.
func (s *Service) notifyAttendees(ctx context.Context, eventID, title, text string) {
	if s.cnuc == nil {
		return
	}

	ids, err := s.repo.AllAttendees(ctx, eventID)
	if err != nil {
		s.logger.Error("failed to list attendees to notify", zap.String("event_id", eventID), zap.Error(err))
		return
	}
	for _, id := range ids {
		if err := s.cnuc.CreateNotification(ctx, title, text, notificationuc.WithUserId(id)); err != nil {
			s.logger.Error("failed to notify attendee", zap.String("event_id", eventID), zap.Error(err))
		}
	}
}

// formatMoscow prints a time as "10.10 в 19:30" in Moscow time.
func formatMoscow(t time.Time) string {
	return t.In(moscow).Format("02.01 в 15:04")
}
