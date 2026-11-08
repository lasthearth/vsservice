package service

import (
	"context"
	"testing"
	"time"

	ticketv1 "github.com/lasthearth/vsservice/gen/ticket/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/ticket/internal/model"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// jobClock is the fixed time the observed services run on.
var jobClock = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

// newLoggedService builds a Service whose logs the test reads back.
func newLoggedService(repo Repository) (*Service, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)
	s := &Service{
		logger:   logger.NewWithCore(core),
		repo:     repo,
		notifier: &fakeNotifier{},
		now:      func() time.Time { return jobClock },
	}
	return s, logs
}

// raceRepo plays a player who answers between the due-list read and the
// update: ListAutoCloseDue moves the first due ticket back to OPEN, so
// model.AutoClose reports false on the fresh document.
type raceRepo struct {
	*fakeRepo
	listed []string
	raced  string
	writes int
}

func (r *raceRepo) ListAutoCloseDue(ctx context.Context, cutoff time.Time, limit int) ([]string, error) {
	ids, err := r.fakeRepo.ListAutoCloseDue(ctx, cutoff, limit)
	if err != nil {
		return nil, err
	}
	r.listed = ids
	if len(ids) > 0 {
		r.raced = ids[0]
		if err := r.tickets[ids[0]].SetStatus(model.StatusOpen, time.Now()); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func (r *raceRepo) Update(ctx context.Context, id string, fn func(context.Context, *model.Ticket) (*model.Ticket, error)) (*model.Ticket, error) {
	out, err := r.fakeRepo.Update(ctx, id, fn)
	if err == nil {
		r.writes++
	}
	return out, err
}

func TestAutoCloseSkipsTicketsThatStoppedQualifying(t *testing.T) {
	repo := &raceRepo{fakeRepo: newFakeRepo()}
	s, logs := newLoggedService(repo)

	for _, uid := range []string{"p1", "p2"} {
		tk := create(t, s, uid)
		if _, err := s.AdminReplyTicket(as("admin"), &ticketv1.AdminReplyTicketRequest{Id: tk.GetId(), Text: "ok"}); err != nil {
			t.Fatal(err)
		}
	}

	// Count only the job's own writes; the setup replies wrote through the
	// same Update.
	repo.writes = 0
	s.CloseStale(context.Background(), jobClock.Add(model.AutoCloseAfter))

	if len(repo.listed) != 2 || repo.raced == "" {
		t.Fatalf("listed = %v, raced = %q", repo.listed, repo.raced)
	}
	raced := repo.tickets[repo.raced]
	if raced.IsClosed() || raced.ClosedAt != nil {
		t.Fatalf("the raced ticket was closed: status %s closed_at %v", raced.Status, raced.ClosedAt)
	}
	if raced.Status != model.StatusOpen {
		t.Fatalf("the raced ticket status = %s, want open", raced.Status)
	}
	// The not-due ticket is refused inside Update, so the fake stores nothing
	// for it: one write for one real close.
	if repo.writes != 1 {
		t.Fatalf("writes = %d, want 1 (only the real close)", repo.writes)
	}
	for id, tk := range repo.tickets {
		if id != repo.raced && !tk.IsClosed() {
			t.Fatalf("the still-due ticket %s was not closed", id)
		}
	}

	// closed counts only real closes, and a not-due ticket is not a failure.
	entries := logs.FilterMessage("closed stale tickets")
	if entries.Len() != 1 {
		t.Fatalf("close summaries = %d, want 1: %+v", entries.Len(), logs.All())
	}
	count, ok := entries.All()[0].ContextMap()["count"].(int64)
	if !ok || count != 1 {
		t.Fatalf("closed count = %v, want 1", entries.All()[0].ContextMap()["count"])
	}
	if n := logs.FilterLevelExact(zapcore.WarnLevel).Len(); n != 0 {
		t.Fatalf("a skipped ticket logged %d warnings", n)
	}
	if n := logs.FilterLevelExact(zapcore.ErrorLevel).Len(); n != 0 {
		t.Fatalf("a skipped ticket logged %d errors", n)
	}
}

// cancelRepo fails the due-list read the way a shutdown cancels the job
// context mid-pass.
type cancelRepo struct{ *fakeRepo }

func (r *cancelRepo) ListAutoCloseDue(context.Context, time.Time, int) ([]string, error) {
	return nil, context.Canceled
}

func TestCloseStaleReturnsSilentlyOnCancellation(t *testing.T) {
	s, logs := newLoggedService(&cancelRepo{newFakeRepo()})

	// Must return: no panic, no error path, no log.
	s.CloseStale(context.Background(), jobClock)

	if n := logs.FilterLevelExact(zapcore.ErrorLevel).Len(); n != 0 {
		t.Fatalf("a canceled pass logged %d errors: %+v", n, logs.All())
	}
	if n := logs.Len(); n != 0 {
		t.Fatalf("a canceled pass must stay silent, got %d entries: %+v", n, logs.All())
	}
}
