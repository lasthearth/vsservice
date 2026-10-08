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

func (r *fakeRepo) ListAttendees(_ context.Context, _ string, limit int) ([]string, int64, error) {
	if limit <= 0 || limit > len(r.attendees) {
		limit = len(r.attendees)
	}
	return slices.Clone(r.attendees[:limit]), int64(len(r.attendees)), nil
}

func (r *fakeRepo) ListMine(_ context.Context, _ string, now time.Time, past bool, _ int) ([]model.Event, error) {
	// The player is signed up iff the fake carries attendees; the event's own
	// state decides which list it lands on.
	if len(r.attendees) == 0 {
		return nil, nil
	}
	if !r.event.Until().After(now) != past {
		return nil, nil
	}
	return []model.Event{*r.event}, nil
}

func (r *fakeRepo) SoftDelete(_ context.Context, id, _ string) error {
	if id != r.id {
		return ierror.ErrNotFound
	}
	return nil
}

func (r *fakeRepo) UnclaimReminder(_ context.Context, _, userID string, startsAt time.Time) error {
	if at, ok := r.reminded[userID]; ok && at.Equal(startsAt) {
		delete(r.reminded, userID)
	}
	return nil
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

// fakeNotifier records notifications; the first `fails` attempts fail, to
// exercise the retry paths.
type fakeNotifier struct {
	sent  []sent
	fails int
	tries int
}

func (n *fakeNotifier) CreateNotification(_ context.Context, title, text string, _ ...notificationuc.NotificationOpts) error {
	n.tries++
	if n.tries <= n.fails {
		return errors.New("send failed")
	}
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

// TestSubMillisecondStartChangeDoesNotNotify pins the BSON granularity of the
// move check: a start that differs only below the millisecond round-trips to
// the same stored date, so nobody should hear about a "move" that did not
// happen.
func TestSubMillisecondStartChangeDoesNotNotify(t *testing.T) {
	svc, repo, n := attendService(t)
	repo.attendees = []string{"u1"}

	if _, err := svc.UpdateEvent(context.Background(), &eventv1.UpdateEventRequest{
		Id: "e1", Title: "Ярмарка", StartsAt: timestamppb.New(repo.event.StartsAt.Add(500 * time.Nanosecond)),
	}); err != nil {
		t.Fatal(err)
	}
	if len(n.sent) != 0 {
		t.Fatalf("a sub-millisecond change notified attendees: %+v", n.sent)
	}
}

func TestListAttendees(t *testing.T) {
	svc, repo, _ := attendService(t)
	repo.attendees = []string{"u1", "u2", "u3"}

	got, err := svc.ListAttendees(context.Background(), &eventv1.ListAttendeesRequest{Id: "e1"})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetTotal() != 3 || !slices.Equal(got.GetUserIds(), []string{"u1", "u2", "u3"}) {
		t.Fatalf("attendees %v total %d", got.GetUserIds(), got.GetTotal())
	}

	// A page smaller than the total still reports the true total.
	got, err = svc.ListAttendees(context.Background(), &eventv1.ListAttendeesRequest{Id: "e1", PageSize: 2})
	if err != nil || len(got.GetUserIds()) != 2 || got.GetTotal() != 3 {
		t.Fatalf("page: ids %v total %d err %v", got.GetUserIds(), got.GetTotal(), err)
	}

	if _, err := svc.ListAttendees(context.Background(), &eventv1.ListAttendeesRequest{Id: "nope"}); !errors.Is(err, ierror.ErrNotFound) {
		t.Fatalf("unknown event: want ErrNotFound, got %v", err)
	}
}

func TestListMyEvents(t *testing.T) {
	svc, repo, _ := attendService(t)
	repo.attendees = []string{"u1"}

	got, err := svc.ListMyEvents(as("u1"), &eventv1.ListMyEventsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetEvents()) != 1 || got.GetEvents()[0].GetAttendeeCount() != 1 {
		t.Fatalf("upcoming: %+v", got.GetEvents())
	}

	// The event is over: it leaves the upcoming list and moves to the past one.
	svc.now = func() time.Time { return repo.event.Until() }
	got, err = svc.ListMyEvents(as("u1"), &eventv1.ListMyEventsRequest{Past: true})
	if err != nil || len(got.GetEvents()) != 1 {
		t.Fatalf("past: %+v %v", got.GetEvents(), err)
	}
	got, err = svc.ListMyEvents(as("u1"), &eventv1.ListMyEventsRequest{})
	if err != nil || len(got.GetEvents()) != 0 {
		t.Fatalf("a finished event must leave the upcoming list: %+v", got.GetEvents())
	}

	if _, err := svc.ListMyEvents(context.Background(), &eventv1.ListMyEventsRequest{}); err == nil {
		t.Fatal("a guest must not list their events")
	}
}

func TestDeleteEventNotifiesAttendees(t *testing.T) {
	svc, repo, n := attendService(t)
	repo.attendees = []string{"u1", "u2"}

	if _, err := svc.DeleteEvent(as("admin"), &eventv1.DeleteEventRequest{Id: "e1"}); err != nil {
		t.Fatal(err)
	}
	if len(n.sent) != 2 || n.sent[0].title != "Событие отменено" || !strings.Contains(n.sent[0].text, "Ярмарка") {
		t.Fatalf("cancel notifications: %+v", n.sent)
	}
}

func TestDeleteEventAfterTheEndDoesNotNotify(t *testing.T) {
	svc, repo, n := attendService(t)
	repo.attendees = []string{"u1"}
	// The event is long over: nobody needs to hear about a cancellation.
	svc.now = func() time.Time { return repo.event.Until().Add(time.Hour) }

	if _, err := svc.DeleteEvent(as("admin"), &eventv1.DeleteEventRequest{Id: "e1"}); err != nil {
		t.Fatal(err)
	}
	if len(n.sent) != 0 {
		t.Fatalf("a finished event notified: %+v", n.sent)
	}
}

// TestFailedReminderSendIsRetried pins the claim give-back: without it a send
// that fails after the claim — a rolling deploy cancelling the context mid-loop,
// a transient insert failure — silently costs the player their only reminder.
func TestFailedReminderSendIsRetried(t *testing.T) {
	svc, repo, n := attendService(t)
	repo.attendees = []string{"u1"}
	n.fails = 1
	start := repo.event.StartsAt

	svc.RemindDue(context.Background(), start.Add(-45*time.Minute))
	if len(n.sent) != 0 {
		t.Fatalf("the failing send must not be recorded: %+v", n.sent)
	}

	// The claim was given back, so the next tick retries and delivers.
	svc.RemindDue(context.Background(), start.Add(-44*time.Minute))
	if len(n.sent) != 1 {
		t.Fatalf("the reminder was lost instead of retried: %+v", n.sent)
	}

	// And the retry does not repeat once delivered.
	svc.RemindDue(context.Background(), start.Add(-43*time.Minute))
	if len(n.sent) != 1 {
		t.Fatalf("reminded twice: %+v", n.sent)
	}
}
