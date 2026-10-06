package service

import (
	"time"

	eventv1 "github.com/lasthearth/vsservice/gen/event/v1"
	"github.com/lasthearth/vsservice/internal/event/internal/model"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toProto(e *model.Event) *eventv1.Event {
	out := &eventv1.Event{
		Id:          e.Id,
		Title:       e.Title,
		Description: e.Description,
		Cover:       e.Cover,
		Location:    e.Location,
		StartsAt:    timestamppb.New(e.StartsAt),
		CreatedBy:   e.CreatedBy,
		CreatedAt:   timestamppb.New(e.CreatedAt),
		UpdatedAt:   timestamppb.New(e.UpdatedAt),
	}
	if e.EndsAt != nil {
		out.EndsAt = timestamppb.New(*e.EndsAt)
	}
	return out
}

func toProtos(events []model.Event) []*eventv1.Event {
	out := make([]*eventv1.Event, 0, len(events))
	for i := range events {
		out = append(out, toProto(&events[i]))
	}
	return out
}

func timePtr(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	t := ts.AsTime()
	return &t
}

func timeOf(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}
