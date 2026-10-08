//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/event/internal/service
package service

import (
	"context"
	"time"

	eventv1 "github.com/lasthearth/vsservice/gen/event/v1"
	"github.com/lasthearth/vsservice/internal/event/internal/model"
	"github.com/lasthearth/vsservice/internal/event/internal/repository"
)

// goverter:converter
// goverter:output:file sermapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTimestamp
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimePtrToTimestamp
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimestampToTime
// goverter:extend github.com/lasthearth/vsservice/internal/event/internal/goverter:TimestampToTimePtr
type Mapper interface {
	// goverter:ignore state sizeCache unknownFields
	// goverter:ignore AttendeeCount AttendeePreview
	ToProto(model.Event) *eventv1.Event
	ToProtos([]model.Event) []*eventv1.Event

	// goverter:useZeroValueOnPointerInconsistency
	CreateRequestToDetails(*eventv1.CreateEventRequest) model.Details
	// goverter:useZeroValueOnPointerInconsistency
	UpdateRequestToDetails(*eventv1.UpdateEventRequest) model.Details
}

var _ Repository = (*repository.Repository)(nil)

// Repository stores events.
type Repository interface {
	Create(ctx context.Context, event *model.Event) (*model.Event, error)
	Get(ctx context.Context, id string) (*model.Event, error)
	UpdateEvent(
		ctx context.Context,
		id string,
		updateFn func(ctx context.Context, e *model.Event) (*model.Event, error),
	) (*model.Event, error)
	SoftDelete(ctx context.Context, id, deletedBy string) error
	ListUpcoming(ctx context.Context, now time.Time, limit int) ([]model.Event, error)
	ListPast(ctx context.Context, now time.Time, limit int) ([]model.Event, error)

	SetAttendance(ctx context.Context, eventID, userID string, attending bool, now time.Time) error
	Attendees(ctx context.Context, eventIDs []string) (map[string]model.Attendees, error)
	ListAttendees(ctx context.Context, eventID string, limit int) ([]string, int64, error)
	AllAttendees(ctx context.Context, eventID string) ([]string, error)
	ListMine(ctx context.Context, userID string, now time.Time, past bool, limit int) ([]model.Event, error)
	ListStartingWithin(ctx context.Context, from, to time.Time) ([]model.Event, error)
	ClaimReminders(ctx context.Context, eventID string, startsAt time.Time) ([]string, error)
}
