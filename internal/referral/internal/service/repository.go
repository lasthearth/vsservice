package service

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/referral/internal/model"
)

// Repository defines persistence operations for referral codes and events.
type Repository interface {
	GetCodeByPlayerID(ctx context.Context, playerID string) (*model.ReferralCode, error)
	GetCodeByCode(ctx context.Context, code string) (*model.ReferralCode, error)
	// UpsertCode inserts code if no referral code exists yet for its
	// player_id, and returns the document that ended up persisted. If a
	// concurrent call already inserted a code for the same player_id, the
	// returned code reflects that winning document rather than the input.
	UpsertCode(ctx context.Context, code *model.ReferralCode) (*model.ReferralCode, error)
	CreateEvent(ctx context.Context, event *model.ReferralEvent) error
	HasReferee(ctx context.Context, refereePlayerID string) (bool, error)
	GetStatsByPlayerID(ctx context.Context, playerID string) (totalReferrals int64, totalCoins int64, err error)

	// ListUnpaidEvents returns up to limit events that still owe a reward and are
	// due for a look at now. An event leaves the list only when every stage of it
	// is paid, so a stage that failed stays visible on the next pass.
	ListUnpaidEvents(ctx context.Context, now time.Time, limit int64) ([]*model.ReferralEvent, error)
	// DeferEvent persists when this event may be looked at again. A pass that finds
	// nothing to pay calls it after the model moved the schedule, so a referral
	// waiting on a player who never arrives does not fill every pass.
	DeferEvent(ctx context.Context, event *model.ReferralEvent) error
	// ClaimReward atomically marks one stage of refereePlayerID's event as paid
	// and, for the referrer's stages, adds coins to the event's running total. It
	// returns false when the stage is already claimed, so only one pass — or one
	// instance — pays it.
	ClaimReward(ctx context.Context, refereePlayerID string, stage model.RewardStage, coins int64) (bool, error)
	// ReleaseReward undoes ClaimReward after the payout failed, so the next pass
	// tries the stage again.
	ReleaseReward(ctx context.Context, refereePlayerID string, stage model.RewardStage, coins int64) error
	// GetRefereeProgress reports how far the referee has got: whether they passed
	// verification on the site, and whether they have ever joined the game server.
	GetRefereeProgress(ctx context.Context, refereePlayerID string) (verified, joined bool, err error)
}
