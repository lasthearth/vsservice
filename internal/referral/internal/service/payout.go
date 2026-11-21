package service

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/donate/donateuc"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/referral/internal/model"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	// defaultPayoutInterval is what a zero REFERRAL_PAYOUT_INTERVAL falls back
	// to, because time.NewTicker panics on a non-positive interval.
	defaultPayoutInterval = time.Minute

	// payoutTimeout bounds one pass. A pass that hangs on Mongo would otherwise
	// hold the loop until the next tick piles up behind it.
	payoutTimeout = 30 * time.Second

	// payoutBatch caps how many events one pass works through, so a backlog is
	// paid in order over several passes instead of one slow pass.
	payoutBatch = 200

	// payoutJob names the loop in its records, and the event keys below mark one
	// pass, one paid reward and one reward left for the next tick. The loop runs
	// every minute, so its records are the only way to tell "nothing was owed"
	// from "the loop stopped running".
	payoutJob     = "referral.payout"
	payoutOK      = "referral.payout.ok"
	payoutFailed  = "referral.payout.failed"
	payoutPaid    = "referral.payout.paid"
	payoutRetried = "referral.payout.retried"
)

// PayoutWorker pays the referral rewards whose moment has come: the referrer's
// first share when the referee passes verification, the rest of the referrer's
// reward and the referee's own reward when the referee first joins the game
// server.
//
// It is a loop rather than a hook because the two moments belong to other
// domains: an admin approval in internal/player and the game server writing
// presence straight into Mongo. Reading the referee's player document from here
// costs one loop and no dependency between the domains, and it retries a failed
// payout by itself.
type PayoutWorker struct {
	repo     Repository
	donateUC *donateuc.AddCoinsUseCase
	log      logger.Logger
	interval time.Duration

	// now is a field so a test can pin the clock a pass is dated by.
	now func() time.Time
}

// PayoutOpts are the worker's dependencies.
type PayoutOpts struct {
	fx.In
	Log      logger.Logger
	Cfg      config.Config
	DbRepo   Repository
	DonateUC *donateuc.AddCoinsUseCase
}

// NewPayoutWorker builds the loop.
func NewPayoutWorker(opts PayoutOpts) *PayoutWorker {
	interval := opts.Cfg.ReferralPayoutInterval
	if interval <= 0 {
		interval = defaultPayoutInterval
	}

	return &PayoutWorker{
		repo:     opts.DbRepo,
		donateUC: opts.DonateUC,
		log:      opts.Log.WithComponent("referral-payout"),
		interval: interval,
		now:      time.Now,
	}
}

// Run performs the first pass and then one per interval until ctx is done. fx
// starts it from OnStart. Every failure is logged and retried on the next tick:
// a reward that is a minute late is better than a process that dies with a
// referee's coins unpaid.
func (w *PayoutWorker) Run(ctx context.Context) {
	w.Pass(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.Pass(ctx)
		}
	}
}

// Pass is one run over the referrals that still owe a reward.
func (w *PayoutWorker) Pass(ctx context.Context) {
	// One pass is one job: the loop has no request to inherit a correlation id
	// from, so it mints one and every record of this pass carries it.
	ctx, _ = logger.WithJob(ctx, payoutJob)
	log := logger.From(ctx, w.log).WithMethod("Pass")

	ctx, cancel := context.WithTimeout(ctx, payoutTimeout)
	defer cancel()

	start := time.Now()
	now := w.now()

	events, err := w.repo.ListUnpaidEvents(ctx, now, payoutBatch)
	if err != nil {
		log.Warn("failed to list the referrals that still owe a reward, retrying on the next tick",
			zap.String("event", payoutFailed),
			zap.Error(err))
		return
	}

	for _, event := range events {
		w.payEvent(ctx, log, event, now)
	}

	if len(events) == 0 {
		// An idle pass is a heartbeat, not news: at Info it would be 1440 records
		// a day per instance saying nothing happened.
		log.Debug("nothing owed to any referral", zap.String("event", payoutOK))
		return
	}

	log.Info("referral payout pass finished",
		zap.String("event", payoutOK),
		zap.Int("events", len(events)),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()))
}

// payEvent pays every reward event still owes, in the order the model lists
// them. Each stage is claimed before its coins move, so a stage that is paid
// here cannot be paid again by the next pass or by another instance.
func (w *PayoutWorker) payEvent(
	ctx context.Context,
	log logger.Logger,
	event *model.ReferralEvent,
	now time.Time,
) {
	verified, joined, err := w.repo.GetRefereeProgress(ctx, event.RefereePlayerID)
	if err != nil {
		log.Warn("failed to read how far the referee got, retrying on the next tick",
			zap.String("event", payoutFailed),
			zap.String("referee_player_id", event.RefereePlayerID),
			zap.Error(err))
		return
	}

	rewards := event.PendingRewards(verified, joined)
	if len(rewards) == 0 {
		w.deferEvent(ctx, log, event, now)
		return
	}

	for _, reward := range rewards {
		claimed, err := w.repo.ClaimReward(ctx, event.RefereePlayerID, reward.Stage, reward.Coins)
		if err != nil {
			log.Warn("failed to claim a referral reward, retrying on the next tick",
				zap.String("event", payoutFailed),
				zap.String("referee_player_id", event.RefereePlayerID),
				zap.String("stage", string(reward.Stage)),
				zap.Error(err))
			continue
		}
		if !claimed {
			// Another pass claimed it first and is paying or has paid it.
			continue
		}

		if err := w.donateUC.Credit(
			ctx,
			reward.Payee,
			"",
			reward.Coins,
			rewardReason(event, reward),
		); err != nil {
			w.release(ctx, log, event, reward, err)
			continue
		}

		log.Info("referral reward paid",
			zap.String("event", payoutPaid),
			zap.String("referee_player_id", event.RefereePlayerID),
			zap.String("payee_player_id", reward.Payee),
			zap.String("stage", string(reward.Stage)),
			zap.Int64("coins", reward.Coins))
	}
}

// deferEvent moves the event's next look forward when this pass found nothing to
// pay: the referee has not reached the next stage yet, or the stage is
// configured to pay nothing. Without it every referral whose player never shows
// up would be looked at on every pass and would hide the ones that just moved.
func (w *PayoutWorker) deferEvent(
	ctx context.Context,
	log logger.Logger,
	event *model.ReferralEvent,
	now time.Time,
) {
	event.DeferNext(now)

	if err := w.repo.DeferEvent(ctx, event); err != nil {
		log.Warn("failed to defer a referral that owes nothing yet, it is looked at again on the next tick",
			zap.String("event", payoutFailed),
			zap.String("referee_player_id", event.RefereePlayerID),
			zap.Error(err))
	}
}

// release gives a claimed stage back after its payout failed, so the next pass
// tries it again. The release runs on a detached context: the failure that
// stopped the payout (a cancelled request among them) must not stop the undo
// too.
//
// A release after a Credit that credited the wallet and then failed to write its
// ledger row would pay that stage twice. The window is the two Mongo writes
// inside Credit wide, and this Error line names the referee and the stage, so a
// duplicate is findable. Leaving the claim standing instead would make a wallet
// that was briefly unreachable swallow the reward for good — the failure this
// loop exists to prevent.
func (w *PayoutWorker) release(
	ctx context.Context,
	log logger.Logger,
	event *model.ReferralEvent,
	reward model.Reward,
	cause error,
) {
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), payoutTimeout)
	defer cancel()

	if err := w.repo.ReleaseReward(releaseCtx, event.RefereePlayerID, reward.Stage, reward.Coins); err != nil {
		log.Error("referral reward failed and its stage could not be released, the reward stays unpaid until an admin pays it",
			zap.String("event", payoutFailed),
			zap.String("referee_player_id", event.RefereePlayerID),
			zap.String("payee_player_id", reward.Payee),
			zap.String("stage", string(reward.Stage)),
			zap.Int64("coins", reward.Coins),
			zap.NamedError("cause", cause),
			zap.Error(err))
		return
	}

	log.Warn("referral reward failed, the stage was released and is retried on the next tick",
		zap.String("event", payoutRetried),
		zap.String("referee_player_id", event.RefereePlayerID),
		zap.String("payee_player_id", reward.Payee),
		zap.String("stage", string(reward.Stage)),
		zap.Int64("coins", reward.Coins),
		zap.NamedError("cause", cause))
}

// rewardReason is the donate ledger row's reason. It names the referee for the
// referrer's stages and the referrer for the referee's own, so a coin in a
// wallet can be traced back to the referral that owed it.
func rewardReason(event *model.ReferralEvent, reward model.Reward) string {
	switch reward.Stage {
	case model.StageVerification:
		return "referral: referrer share for the verification of " + event.RefereePlayerID
	case model.StageJoin:
		return "referral: referrer share for the first join of " + event.RefereePlayerID
	case model.StageReferee:
		return "referral: invited player reward from " + event.ReferrerPlayerID
	default:
		return "referral reward"
	}
}
