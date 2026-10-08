package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	eventv1 "github.com/lasthearth/vsservice/gen/event/v1"
	"github.com/lasthearth/vsservice/internal/event/internal/ierror"
	"github.com/lasthearth/vsservice/internal/event/internal/model"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (r *fakeRepo) Get(_ context.Context, id string) (*model.Event, error) {
	if id != r.id {
		return nil, ierror.ErrNotFound
	}
	e := *r.event
	return &e, nil
}

func (r *fakeRepo) SetAttendance(_ context.Context, _, userID string, attending bool, _ time.Time) error {
	in := slices.Contains(r.attendees, userID)
	switch {
	case attending && !in:
		r.attendees = append(r.attendees, userID)
	case !attending && in:
		r.attendees = slices.DeleteFunc(r.attendees, func(id string) bool { return id == userID })
	}
	return nil
}

// Attendees reports the one fake event's sign-ups under every requested id
// (the fake's events carry no id).
func (r *fakeRepo) Attendees(_ context.Context, ids []string) (map[string]model.Attendees, error) {
	out := map[string]model.Attendees{}
	if len(r.attendees) == 0 {
		return out, nil
	}
	preview := r.attendees[:min(len(r.attendees), model.AttendeePreviewSize)]
	for _, id := range ids {
		out[id] = model.Attendees{Count: int32(len(r.attendees)), Preview: slices.Clone(preview)}
	}
	return out, nil
}

func (r *fakeRepo) AllAttendees(context.Context, string) ([]string, error) {
	return slices.Clone(r.attendees), nil
}

func (r *fakeRepo) ListStartingWithin(_ context.Context, from, to time.Time) ([]model.Event, error) {
	if r.event.StartsAt.After(from) && !r.event.StartsAt.After(to) {
		return []model.Event{*r.event}, nil
	}
	return nil, nil
}

func (r *fakeRepo) ClaimReminders(_ context.Context, _ string, startsAt time.Time) ([]string, error) {
	if r.reminded == nil {
		r.reminded = map[string]time.Time{}
	}
	var claimed []string
	for _, id := range r.attendees {
		if at, ok := r.reminded[id]; ok && at.Equal(startsAt) {
			continue
		}
		r.reminded[id] = startsAt
		claimed = append(claimed, id)
	}
	return claimed, nil
}

// sent is a notification captured by fakeNotifier.
type sent struct{ title, text string }

type fakeNotifier struct{ sent []sent }

func (n *fakeNotifier) CreateNotification(_ context.Context, title, text string, _ ...notificationuc.NotificationOpts) error {
	n.sent = append(n.sent, sent{title, text})
	return nil
}

func as(uid string) context.Context {
	return interceptor.ContextWithUserID(context.Background(), uid)
}

// attendService returns a service whose clock stands two hours before the
// test event (18:00–21:00 UTC on 10 Oct).
func attendService(t *testing.T) (*Service, *fakeRepo, *fakeNotifier) {
	t.Helper()
	svc, repo := newTestService(t)
	n := &fakeNotifier{}
	svc.cnuc = n
	svc.now = func() time.Time { return repo.event.StartsAt.Add(-2 * time.Hour) }
	return svc, repo, n
}

func TestSetAttendanceSignsUpAndWithdraws(t *testing.T) {
	svc, repo, _ := attendService(t)

	got, err := svc.SetAttendance(as("u1"), &eventv1.SetAttendanceRequest{Id: "e1", Attending: true})
	if err != nil || !got.GetAttending() || got.GetAttendeeCount() != 1 {
		t.Fatalf("sign up: %+v %v", got, err)
	}
	// Idempotent: signing up again changes nothing.
	got, _ = svc.SetAttendance(as("u1"), &eventv1.SetAttendanceRequest{Id: "e1", Attending: true})
	if got.GetAttendeeCount() != 1 {
		t.Fatalf("second sign-up counted: %d", got.GetAttendeeCount())
	}
	_, _ = svc.SetAttendance(as("u2"), &eventv1.SetAttendanceRequest{Id: "e1", Attending: true})

	got, err = svc.SetAttendance(as("u1"), &eventv1.SetAttendanceRequest{Id: "e1", Attending: false})
	if err != nil || got.GetAttending() || got.GetAttendeeCount() != 1 || got.GetAttendeePreview()[0] != "u2" {
		t.Fatalf("withdraw: %+v %v", got, err)
	}
	if len(repo.attendees) != 1 {
		t.Fatalf("stored attendees %v", repo.attendees)
	}
}

func TestSetAttendanceRules(t *testing.T) {
	svc, repo, _ := attendService(t)

	if _, err := svc.SetAttendance(context.Background(), &eventv1.SetAttendanceRequest{Id: "e1", Attending: true}); err == nil {
		t.Fatal("a guest must not sign up")
	}
	if _, err := svc.SetAttendance(as("u1"), &eventv1.SetAttendanceRequest{Id: "nope", Attending: true}); !errors.Is(err, ierror.ErrNotFound) {
		t.Fatalf("unknown event: want ErrNotFound, got %v", err)
	}

	svc.now = func() time.Time { return repo.event.Until() }
	if _, err := svc.SetAttendance(as("u1"), &eventv1.SetAttendanceRequest{Id: "e1", Attending: true}); !errors.Is(err, model.ErrEventOver) {
		t.Fatalf("finished event: want ErrEventOver, got %v", err)
	}
}

func TestEventsCarryAttendees(t *testing.T) {
	svc, repo, _ := attendService(t)
	repo.attendees = []string{"a", "b", "c", "d", "e", "f", "g"}

	got, err := svc.GetEvent(context.Background(), &eventv1.GetEventRequest{Id: "e1"})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetAttendeeCount() != 7 || len(got.GetAttendeePreview()) != model.AttendeePreviewSize {
		t.Fatalf("count %d preview %v", got.GetAttendeeCount(), got.GetAttendeePreview())
	}
}

func TestRemindDueOncePerStart(t *testing.T) {
	svc, repo, n := attendService(t)
	repo.attendees = []string{"u1", "u2"}
	start := repo.event.StartsAt
	end := *repo.event.EndsAt
	if err := repo.event.Apply(model.Details{Title: "Ярмарка", Location: "у моста", StartsAt: start, EndsAt: &end}); err != nil {
		t.Fatal(err)
	}

	// Two hours before: too early.
	svc.RemindDue(context.Background(), start.Add(-2*time.Hour))
	if len(n.sent) != 0 {
		t.Fatalf("reminded too early: %v", n.sent)
	}

	svc.RemindDue(context.Background(), start.Add(-45*time.Minute))
	if len(n.sent) != 2 || !strings.Contains(n.sent[0].text, "через 45 мин") || !strings.Contains(n.sent[0].text, "у моста") {
		t.Fatalf("reminders: %+v", n.sent)
	}

	// The next tick does not repeat them.
	svc.RemindDue(context.Background(), start.Add(-44*time.Minute))
	if len(n.sent) != 2 {
		t.Fatalf("reminded twice: %d", len(n.sent))
	}
}

func TestMovedEventNotifiesAndRemindsAgain(t *testing.T) {
	svc, repo, n := attendService(t)
	repo.attendees = []string{"u1"}
	start := repo.event.StartsAt

	svc.RemindDue(context.Background(), start.Add(-30*time.Minute))
	if len(n.sent) != 1 {
		t.Fatalf("first reminder: %v", n.sent)
	}

	// Moved a day later: attendees hear about it, and get a fresh reminder.
	moved := start.Add(24 * time.Hour)
	if _, err := svc.UpdateEvent(context.Background(), &eventv1.UpdateEventRequest{
		Id: "e1", Title: "Ярмарка", StartsAt: timestamppb.New(moved),
	}); err != nil {
		t.Fatal(err)
	}
	if len(n.sent) != 2 || n.sent[1].title != "Событие перенесено" {
		t.Fatalf("move notification: %+v", n.sent)
	}

	svc.RemindDue(context.Background(), moved.Add(-30*time.Minute))
	if len(n.sent) != 3 || n.sent[2].title != "Скоро событие" {
		t.Fatalf("reminder after move: %+v", n.sent)
	}
}

func TestUnchangedStartDoesNotNotify(t *testing.T) {
	svc, repo, n := attendService(t)
	repo.attendees = []string{"u1"}

	if _, err := svc.UpdateEvent(context.Background(), &eventv1.UpdateEventRequest{
		Id: "e1", Title: "Ярмарка у реки", StartsAt: timestamppb.New(repo.event.StartsAt),
	}); err != nil {
		t.Fatal(err)
	}
	if len(n.sent) != 0 {
		t.Fatalf("title edit notified attendees: %+v", n.sent)
	}
}
