package serverinfo

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	// defaultStaleAfter is what SERVERINFO_STALE_AFTER falls back to when the
	// environment says nothing or says zero. It is repeated here so a Sweeper
	// built from a zero config still behaves like a deployment.
	defaultStaleAfter = 30 * time.Second

	// sweepTimeout bounds one pass. Without it a single hung Mongo call stalls
	// the loop while the staleness window slides forward, and the site keeps
	// showing an online count nobody is alive to correct — with no error
	// anywhere.
	sweepTimeout = 15 * time.Second

	// sweepJob names the loop in its records, and sweepOK/sweepFailed are the
	// stable event keys of one pass. A pass runs every 30 seconds, so its
	// records are the only way to tell "nothing was stale" from "the loop
	// stopped running".
	sweepJob    = "serverinfo.sweep"
	sweepOK     = "serverinfo.sweep.ok"
	sweepFailed = "serverinfo.sweep.failed"
)

// Sweeper zeroes a total_online whose heartbeat went stale. The game server owns
// the document and stops writing when it dies, so without this the site would
// keep reporting the online count of a server that is gone.
type Sweeper struct {
	repo       Repository
	log        logger.Logger
	staleAfter time.Duration

	// now is a field so a test can pin the clock the cutoff is taken from.
	now func() time.Time
}

// NewSweeper builds the sweeper. A zero or unset SERVERINFO_STALE_AFTER falls
// back to defaultStaleAfter: an interval of zero would make time.NewTicker panic,
// and a stale window of zero would zero the count of a live server.
func NewSweeper(log logger.Logger, repo Repository, cfg config.Config) *Sweeper {
	staleAfter := cfg.ServerInfoStaleAfter
	if staleAfter <= 0 {
		staleAfter = defaultStaleAfter
	}

	return &Sweeper{
		repo:       repo,
		log:        log.WithComponent("serverinfo-sweeper"),
		staleAfter: staleAfter,
		now:        time.Now,
	}
}

// Run performs the first pass and then one per staleAfter until ctx is done.
//
// The tick is the stale window itself: there is no separate interval knob, and
// checking more often than the window is wide cannot clear anything sooner. A
// count that goes stale is therefore zeroed within two windows at worst.
func (s *Sweeper) Run(ctx context.Context) {
	s.Sweep(ctx, s.now())

	ticker := time.NewTicker(s.staleAfter)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Sweep(ctx, s.now())
		}
	}
}

// Sweep is one pass. A failure is logged and left for the next tick: the sweeper
// is a janitor, and a janitor that can take the process down with it is worse
// than a stale count for another 30 seconds.
func (s *Sweeper) Sweep(ctx context.Context, now time.Time) {
	// One pass is one job: the loop has no request to inherit a correlation id
	// from, so it mints one and every record of this pass carries it.
	ctx, _ = logger.WithJob(ctx, sweepJob)
	log := logger.From(ctx, s.log).WithMethod("sweep")

	ctx, cancel := context.WithTimeout(ctx, sweepTimeout)
	defer cancel()

	start := time.Now()

	cleared, err := s.repo.ClearStaleOnline(ctx, now.Add(-s.staleAfter).UnixMilli())
	if err != nil {
		log.Warn("failed to clear the stale online count, retrying on the next tick",
			zap.String("event", sweepFailed),
			zap.Error(err))
		return
	}

	fields := []zap.Field{
		zap.Int64("n", cleared),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if cleared == 0 {
		// An idle pass is a heartbeat, not news: at Info it would be 2880
		// records a day per instance saying nothing happened.
		log.Debug("serverinfo sweep idle", append(fields, zap.String("event", sweepOK))...)
		return
	}

	log.Info("serverinfo sweep zeroed a stale online count", append(fields, zap.String("event", sweepOK))...)
}
