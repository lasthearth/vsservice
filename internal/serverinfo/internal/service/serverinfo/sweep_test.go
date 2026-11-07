package serverinfo

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/serverinfo/internal/model"
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

func (r *fakeRepo) GetServerInfo(context.Context) (*model.ServerInfo, error) {
	return nil, errors.New("not reached")
}

func (r *fakeRepo) ClearStaleOnline(_ context.Context, cutoffUnixMilli int64) (int64, error) {
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

var _ Repository = (*fakeRepo)(nil)

func newSweeper(t *testing.T, cfg config.Config, repo Repository) *Sweeper {
	t.Helper()

	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	return NewSweeper(l, repo, cfg)
}

// TestNewSweeperFallsBackToTheRolloutDefault pins the zero config. A window of
// zero would be worse than no sweeper at all: it would zero the count of a live
// server whose heartbeat is a millisecond old. And Run ticks on the same value,
// so zero would also panic time.NewTicker.
func TestNewSweeperFallsBackToTheRolloutDefault(t *testing.T) {
	s := newSweeper(t, config.Config{}, newFakeRepo())

	if s.staleAfter != 30*time.Second {
		t.Errorf("staleAfter = %v, want 30s", s.staleAfter)
	}
}

func TestNewSweeperTakesTheConfiguredWindow(t *testing.T) {
	s := newSweeper(t, config.Config{ServerInfoStaleAfter: 10 * time.Second}, newFakeRepo())

	if s.staleAfter != 10*time.Second {
		t.Errorf("staleAfter = %v, want 10s", s.staleAfter)
	}
}

// TestSweepCutsOffOneStaleWindowBehindNow is the arithmetic the sweeper rests
// on. The game writes heartbeat_at every 5 s, so a cutoff of now would zero a
// live server's count on every pass.
func TestSweepCutsOffOneStaleWindowBehindNow(t *testing.T) {
	repo := newFakeRepo()
	s := newSweeper(t, config.Config{ServerInfoStaleAfter: 45 * time.Second}, repo)

	now := time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)
	s.Sweep(context.Background(), now)

	got := repo.got()
	if len(got) != 1 {
		t.Fatalf("sweep asked the repository %d times, want 1", len(got))
	}

	want := now.Add(-45 * time.Second).UnixMilli()
	if got[0] != want {
		t.Errorf("cutoff = %d, want %d (now - staleAfter)", got[0], want)
	}
}

// TestSweepSwallowsAFailedPass pins that a Mongo error never reaches fx startup
// or the goroutine: the sweeper retries on the next tick instead.
func TestSweepSwallowsAFailedPass(t *testing.T) {
	repo := newFakeRepo()
	repo.err = errors.New("mongo is unreachable")
	s := newSweeper(t, config.Config{}, repo)

	s.Sweep(context.Background(), time.Now())

	if len(repo.got()) != 1 {
		t.Fatalf("a failed pass did not reach the repository once: %v", repo.got())
	}
}

// TestRunReturnsWhenTheContextIsCancelled pins the lifecycle contract the fx
// hook relies on: OnStop cancels and then joins the goroutine, so a Run that
// ignored ctx would leave a sweeper running against a closed database.
func TestRunReturnsWhenTheContextIsCancelled(t *testing.T) {
	s := newSweeper(t, config.Config{}, newFakeRepo())

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
