// Package presence keeps verification_requests.is_online honest. The game
// server writes the flag and its heartbeat; this package clears the flag when
// the heartbeat stops arriving.
package presence

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	// defaultSweepInterval and defaultStaleAfter are what PRESENCE_* fall back
	// to when the environment says nothing or says zero. They are repeated here
	// so a Sweeper built from a zero config still behaves like a deployment.
	defaultSweepInterval = time.Minute
	defaultStaleAfter    = time.Minute

	// sweepTimeout bounds one pass. Without it a single hung Mongo call stalls
	// the loop while the staleness window slides forward, and players who left
	// stay online in the roster forever — with no error anywhere.
	sweepTimeout = 15 * time.Second

	// sweepJob names the loop in its records, and sweepOK/sweepFailed are the
	// stable event keys of one pass. A pass runs every minute, so its records
	// are the only way to tell "nobody was stale" from "the loop stopped
	// running".
	sweepJob    = "presence.sweep"
	sweepOK     = "presence.sweep.ok"
	sweepFailed = "presence.sweep.failed"
)

// Repository is the slice of the player repository the sweeper needs. It is
// declared here, next to its only consumer, the way the domain's other
// repository interfaces are.
type Repository interface {
	// ClearStalePresence takes offline every player whose last_heartbeat is
	// older than cutoffUnixMilli, or who carries no last_heartbeat at all, and
	// returns how many documents changed.
	ClearStalePresence(ctx context.Context, cutoffUnixMilli int64) (int64, error)
}

// Sweeper clears the online flag of players whose game server stopped
// heartbeating. It is the only thing that ever takes such a player offline: a
// server that was killed has no disconnect to report.
type Sweeper struct {
	repo       Repository
	log        logger.Logger
	interval   time.Duration
	staleAfter time.Duration

	// now is a field so a test can pin the clock the cutoff is taken from.
	now func() time.Time
}

type Opts struct {
	fx.In
	Repo   Repository
	Logger logger.Logger
	Config config.Config
}

// New builds the sweeper. A zero or unset PRESENCE_SWEEP_INTERVAL falls back to
// defaultSweepInterval, because time.NewTicker panics on a non-positive
// interval; a zero PRESENCE_STALE_AFTER falls back too, because a window of zero
// would take live players offline between two heartbeats.
func New(opts Opts) *Sweeper {
	interval := opts.Config.PresenceSweepInterval
	if interval <= 0 {
		interval = defaultSweepInterval
	}

	staleAfter := opts.Config.PresenceStaleAfter
	if staleAfter <= 0 {
		staleAfter = defaultStaleAfter
	}

	return &Sweeper{
		repo:       opts.Repo,
		log:        opts.Logger.WithComponent("presence-sweeper"),
		interval:   interval,
		staleAfter: staleAfter,
		now:        time.Now,
	}
}

// Run performs the first pass and then one per interval until ctx is done. fx
// starts it from OnStart; every failure is logged and retried on the next tick,
// because the roster must keep serving while Mongo is unreachable.
func (s *Sweeper) Run(ctx context.Context) {
	s.Sweep(ctx, s.now())

	ticker := time.NewTicker(s.interval)
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

// Sweep is one pass. A failure is logged at Warn and left for the next tick:
// the sweeper is a janitor, and one that can take the process down with it is
// worse than a roster that stays wrong for another minute.
func (s *Sweeper) Sweep(ctx context.Context, now time.Time) {
	// One pass is one job: the loop has no request to inherit a correlation id
	// from, so it mints one and every record of this pass carries it.
	ctx, _ = logger.WithJob(ctx, sweepJob)
	log := logger.From(ctx, s.log).WithMethod("sweep")

	ctx, cancel := context.WithTimeout(ctx, sweepTimeout)
	defer cancel()

	start := time.Now()

	cleared, err := s.repo.ClearStalePresence(ctx, now.Add(-s.staleAfter).UnixMilli())
	if err != nil {
		log.Warn("failed to clear the stale online flags, retrying on the next tick",
			zap.String("event", sweepFailed),
			zap.Error(err))
		return
	}

	fields := []zap.Field{
		zap.Int64("n", cleared),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if cleared == 0 {
		// An idle pass is a heartbeat, not news: at Info it would be 1440
		// records a day per instance saying nothing happened.
		log.Debug("presence sweep idle", append(fields, zap.String("event", sweepOK))...)
		return
	}

	log.Info("presence sweep took stale players offline", append(fields, zap.String("event", sweepOK))...)
}
