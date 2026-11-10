package mod_test

import (
	"bytes"
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/mod"
	"github.com/lasthearth/vsservice/internal/mod/internal/model"
	"github.com/lasthearth/vsservice/internal/mod/internal/service"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
	"go.uber.org/zap/zapcore"
)

const (
	// syncInterval is the loop's tick. startBudget is three ticks: the fx start
	// hook context expires while the loop must still run, which is exactly what
	// MODS_SYNC_INTERVAL (1m) against fx's default StartTimeout (15s) does in a
	// deployment.
	syncInterval = 20 * time.Millisecond
	startBudget  = 60 * time.Millisecond
	stopBudget   = 2 * time.Second
	// waitBudget bounds every poll, so a slow machine fails on the count and
	// not on the wait.
	waitBudget = 5 * time.Second
	// passesAcrossStartTimeout is the number of manifest reads the loop must
	// reach: the boot pass plus five ticks, and five ticks cannot fit inside
	// startBudget.
	passesAcrossStartTimeout = 6
)

// stubManifest is one client mod the game server published. It is valid for the
// sync, so every pass reaches the repository write the test counts.
const stubManifest = `{
	"version": 1,
	"generated_at": "2026-01-01T00:00:00Z",
	"game_version": "1.2.3",
	"mods": [{
		"modid": "test-mod",
		"name": "Test mod",
		"version": "1.0.0",
		"side": "universal",
		"required_on_client": true,
		"filename": "test-mod.jar",
		"file_key": "mods/test-mod.jar",
		"size": 10,
		"uploaded_at": "2026-01-01T00:00:00Z"
	}]
}`

// countingObjects stands in for MinIO and counts the manifest reads: one read
// is one sync pass, so the counter is the loop's heartbeat.
type countingObjects struct {
	reads atomic.Int64
}

func (o *countingObjects) GetObject(_ context.Context, _, _ string) (io.ReadCloser, error) {
	o.reads.Add(1)

	return io.NopCloser(bytes.NewReader([]byte(stubManifest))), nil
}

func (o *countingObjects) count() int {
	return int(o.reads.Load())
}

// countingRepo stands in for Mongo and counts the writes a pass applies.
type countingRepo struct {
	syncs atomic.Int64
}

func (r *countingRepo) List(context.Context) ([]model.Mod, error) { return nil, nil }

func (r *countingRepo) Get(context.Context, string) (*model.Mod, error) { return nil, io.EOF }

func (r *countingRepo) Create(context.Context, *model.Mod) error { return nil }

func (r *countingRepo) Save(
	context.Context,
	string,
	func(context.Context, *model.Mod) (*model.Mod, error),
) (*model.Mod, error) {
	return nil, io.EOF
}

func (r *countingRepo) Delete(context.Context, string) error { return nil }

func (r *countingRepo) Reorder(context.Context, []string) error { return nil }

func (r *countingRepo) Sync(context.Context, []model.Mod, []string) error {
	r.syncs.Add(1)

	return nil
}

// readyBuckets makes the startup bootstrap succeed at once: this test is about
// the loop's lifetime, not about a MinIO that is down.
type readyBuckets struct{}

func (readyBuckets) BucketExists(context.Context, string) (bool, error) { return true, nil }

func (readyBuckets) CreateBucket(context.Context, string) error { return nil }

func (readyBuckets) MakeBucketPublic(context.Context, string) error { return nil }

// TestSyncLoopOutlivesStartTimeout runs the real fx lifecycle of mod.App and
// pins the loop's lifetime to the app, not to the start hook.
//
// The fx hook context carries the StartTimeout and is dead shortly after a
// successful start (fx v1.24.0 app.go:606). A loop started on that context
// stops before its first MODS_SYNC_INTERVAL tick, so the stored list goes stale
// silently and never removes a mod the game server dropped. The loop must keep
// ticking after that context expired, and must stop only when the app stops.
func TestSyncLoopOutlivesStartTimeout(t *testing.T) {
	objects := &countingObjects{}
	repo := &countingRepo{}

	app := fx.New(
		fx.NopLogger,
		fx.StartTimeout(startBudget),
		fx.StopTimeout(stopBudget),
		fx.Supply(config.Config{
			ModsBucket:       "test-mods",
			ModsManifestKey:  "manifest.json",
			ModsSyncInterval: syncInterval,
			CdnUrl:           "https://cdn.test",
			ModsArchiveDir:   t.TempDir(),
		}),
		fx.Supply(fx.Annotate(
			logger.NewWithCore(zapcore.NewNopCore()),
			fx.As(new(logger.Logger)),
		)),
		// The module provides its store, its repository and its bucket access
		// privately, so the stubs replace them from the wrapping module: an fx
		// decoration also covers child scopes, and the real Mongo and MinIO
		// constructors never run.
		fx.Module("lifecycle-test",
			fx.Decorate(func() service.ObjectStorage { return objects }),
			fx.Decorate(func() service.Repository { return repo }),
			fx.Decorate(func() service.BucketStore { return readyBuckets{} }),
			mod.App,
		),
	)
	if err := app.Err(); err != nil {
		t.Fatalf("build the app: %v", err)
	}

	// Start gets the budget fx.Run derives from fx.StartTimeout, and that
	// context stays uncancelled after Start returns, exactly like the one
	// app.run hands to the hooks.
	startCtx, cancelStart := context.WithTimeout(context.Background(), app.StartTimeout())
	defer cancelStart()

	if err := app.Start(startCtx); err != nil {
		t.Fatalf("start the app: %v", err)
	}

	waitForReads(t, objects, passesAcrossStartTimeout)

	// The start hook context is dead now. Everything below happens after it.
	if err := startCtx.Err(); err == nil {
		t.Fatal("the start hook context is still alive: the test no longer proves the loop outlives it")
	}

	alive := objects.count()
	waitForReads(t, objects, alive+3)

	stopCtx, cancelStop := context.WithTimeout(context.Background(), stopBudget)
	defer cancelStop()

	if err := app.Stop(stopCtx); err != nil {
		t.Fatalf("stop the app: %v", err)
	}

	stopped := objects.count()

	// OnStop cancels the loop and waits for it, so no tick can follow. Ten
	// intervals of silence is the observable proof that the loop exited.
	time.Sleep(10 * syncInterval)

	if got := objects.count(); got != stopped {
		t.Errorf("the loop kept syncing after Stop: %d reads at stop, %d after", stopped, got)
	}

	if got := repo.syncs.Load(); got == 0 {
		t.Error("no pass wrote the stored list, so the loop never completed a sync")
	}
}

// waitForReads blocks until the loop read the manifest want times.
func waitForReads(t *testing.T, objects *countingObjects, want int) {
	t.Helper()

	deadline := time.Now().Add(waitBudget)

	for objects.count() < want {
		if time.Now().After(deadline) {
			t.Fatalf("the sync loop stopped: %d manifest reads, want %d", objects.count(), want)
		}

		time.Sleep(syncInterval / 4)
	}
}
