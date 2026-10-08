package service

import (
	"context"
	"errors"
	"testing"
	"time"

	eventv1 "github.com/lasthearth/vsservice/gen/event/v1"
	"github.com/lasthearth/vsservice/internal/event/internal/ierror"
	"github.com/lasthearth/vsservice/internal/event/internal/model"
	"github.com/lasthearth/vsservice/internal/event/internal/service/sermapper"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fakeRepo keeps one event and mirrors the repository's callback update:
// the stored event changes only when updateFn succeeds.
type fakeRepo struct {
	Repository
	id    string
	event *model.Event
	// attendees holds sign-ups of e1 in order; reminded maps a user to the
	// start time they were reminded about.
	attendees []string
	reminded  map[string]time.Time
}

func (r *fakeRepo) UpdateEvent(
	ctx context.Context,
	id string,
	updateFn func(ctx context.Context, e *model.Event) (*model.Event, error),
) (*model.Event, error) {
	if id != r.id {
		return nil, ierror.ErrNotFound
	}
	working := *r.event
	updated, err := updateFn(ctx, &working)
	if err != nil {
		return nil, err
	}
	r.event = updated
	return updated, nil
}

func newTestService(t *testing.T) (*Service, *fakeRepo) {
	t.Helper()
	start := time.Date(2026, 10, 10, 18, 0, 0, 0, time.UTC)
	end := start.Add(3 * time.Hour)
	event, err := model.New(model.Details{Title: "Ярмарка", StartsAt: start, EndsAt: &end}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeRepo{id: "e1", event: event}
	return &Service{repo: repo, mapper: &sermapper.MapperImpl{}, now: time.Now}, repo
}

func TestUpdateEventAppliesDetailsThroughModel(t *testing.T) {
	svc, repo := newTestService(t)
	start := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

	got, err := svc.UpdateEvent(context.Background(), &eventv1.UpdateEventRequest{
		Id:       "e1",
		Title:    "  Осада  ",
		StartsAt: timestamppb.New(start),
	})
	if err != nil {
		t.Fatal(err)
	}

	if got.GetTitle() != "Осада" || repo.event.Title != "Осада" {
		t.Fatalf("title not applied: proto %q, stored %q", got.GetTitle(), repo.event.Title)
	}
	if !repo.event.StartsAt.Equal(start) {
		t.Fatalf("starts_at not applied: %v", repo.event.StartsAt)
	}
	// A missing ends_at clears the end instead of becoming the Unix epoch.
	if repo.event.EndsAt != nil || got.GetEndsAt() != nil {
		t.Fatalf("ends_at should be cleared, got %v", repo.event.EndsAt)
	}
}

func TestUpdateEventInvalidDetailsLeaveEventUntouched(t *testing.T) {
	svc, repo := newTestService(t)
	before := *repo.event

	_, err := svc.UpdateEvent(context.Background(), &eventv1.UpdateEventRequest{
		Id:       "e1",
		Title:    "   ",
		StartsAt: timestamppb.New(time.Now()),
	})
	if !errors.Is(err, model.ErrTitleRequired) {
		t.Fatalf("want ErrTitleRequired, got %v", err)
	}
	if repo.event.Title != before.Title || !repo.event.StartsAt.Equal(before.StartsAt) {
		t.Fatal("stored event changed after a failed update")
	}
}

func TestUpdateEventNotFound(t *testing.T) {
	svc, _ := newTestService(t)

	_, err := svc.UpdateEvent(context.Background(), &eventv1.UpdateEventRequest{
		Id:       "missing",
		Title:    "Осада",
		StartsAt: timestamppb.New(time.Now()),
	})
	if !errors.Is(err, ierror.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
