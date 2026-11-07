package presence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap"
)

// fakeRepo records the cutoffs the sweeper asks for and signals every call, so a
// test can wait for a pass without polling a slice the loop is writing.
type fakeRepo struct {
	mu      sync.Mutex
	cutoffs []int64
	cleared int64
	err     error

	passes chan struct{}
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{passes: make(chan struct{}, 8)}
}

func (r *fakeRepo) ClearStalePresence(_ context.Context, cutoffUnixMilli int64) (int64, error) {
	r.mu.Lock()
	r.cutoffs = append(r.cutoffs, cutoffUnixMilli)
	cleared, err := r.cleared, r.err
	r.mu.Unlock()

	select {
	case r.passes <- struct{}{}:
	default:
	}

	return cleared, err
}

func (r *fakeRepo) got() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]int64(nil), r.cutoffs...)
}

func newSweeper(t *testing.T, cfg config.Config, repo Repository) *Sweeper {
	t.Helper()

	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	return New(Opts{Repo: repo, Logger: l, Config: cfg})
}

// TestNewFallsBackToTheRolloutDefaults pins the zero config. envconfig already
// refuses to hand over a zero for an unset variable, but a Sweeper built from a
// zero Config — a test, a future caller that forgets — must still not panic on
// time.NewTicker(0) or take live players offline with a window of zero.
func TestNewFallsBackToTheRolloutDefaults(t *testing.T) {
	s := newSweeper(t, config.Config{}, newFakeRepo())

	if s.interval != time.Minute {
		t.Errorf("interval = %v, want 1m", s.interval)
	}
	if s.staleAfter != time.Minute {
		t.Errorf("staleAfter = %v, want 1m", s.staleAfter)
	}
}

func TestNewTakesTheConfiguredWindow(t *testing.T) {
	s := newSweeper(t, config.Config{
		PresenceSweepInterval: 5 * time.Second,
		PresenceStaleAfter:    20 * time.Second,
	}, newFakeRepo())

	if s.interval != 5*time.Second {
		t.Errorf("interval = %v, want 5s", s.interval)
	}
	if s.staleAfter != 20*time.Second {
		t.Errorf("staleAfter = %v, want 20s", s.staleAfter)
	}
}

// TestSweepCutsOffOneStaleWindowBehindNow is the arithmetic the whole sweeper
// rests on. A cutoff of now would take every online player offline on the next
// tick; a cutoff that ignored staleAfter would never take anyone offline. The
// game beats every 15 s, so the boundary has to be a window behind the caller's
// clock, not the clock itself.
func TestSweepCutsOffOneStaleWindowBehindNow(t *testing.T) {
	repo := newFakeRepo()
	s := newSweeper(t, config.Config{PresenceStaleAfter: 90 * time.Second}, repo)

	now := time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)
	s.Sweep(context.Background(), now)

	got := repo.got()
	if len(got) != 1 {
		t.Fatalf("sweep asked the repository %d times, want 1", len(got))
	}

	want := now.Add(-90 * time.Second).UnixMilli()
	if got[0] != want {
		t.Errorf("cutoff = %d, want %d (now - staleAfter)", got[0], want)
	}
}

// TestSweepSwallowsAFailedPass pins that a Mongo error never reaches fx startup
// or the goroutine: the sweeper is a janitor, and one that takes the process
// down with it is worse than a roster that stays wrong for another minute.
func TestSweepSwallowsAFailedPass(t *testing.T) {
	repo := newFakeRepo()
	repo.err = errors.New("mongo is unreachable")
	s := newSweeper(t, config.Config{}, repo)

	s.Sweep(context.Background(), time.Now())

	if len(repo.got()) != 1 {
		t.Fatalf("a failed pass did not reach the repository once: %v", repo.got())
	}
}

// TestRunSweepsBeforeTheFirstTick pins that the loop does not wait a full
// interval before its first pass. The rollout depends on it: the documents that
// are stuck online have no heartbeat at all, and the deploy that fixes them is
// the one whose first pass has to clear them.
func TestRunSweepsBeforeTheFirstTick(t *testing.T) {
	repo := newFakeRepo()
	s := newSweeper(t, config.Config{PresenceSweepInterval: time.Hour}, repo)
	s.now = func() time.Time { return time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC) }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()

	select {
	case <-repo.passes:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("Run did not sweep until its first ticker tick")
	}

	cancel()
	<-done
}

// TestRunReturnsWhenTheContextIsCancelled pins the lifecycle contract the fx
// hook relies on: OnStop cancels and then joins the goroutine, so a Run that
// ignored ctx would leave a sweeper running against a closed database.
func TestRunReturnsWhenTheContextIsCancelled(t *testing.T) {
	s := newSweeper(t, config.Config{PresenceSweepInterval: time.Hour}, newFakeRepo())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after its context was cancelled")
	}
}
