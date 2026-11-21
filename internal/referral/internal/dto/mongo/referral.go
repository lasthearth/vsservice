package dto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
)

type ReferralCode struct {
	mongox.Model `bson:",inline"`
	PlayerID     string `bson:"player_id"`
	PlayerName   string `bson:"player_name"`
	Code         string `bson:"code"`
}

// ReferralEvent is one referral and the state of its payout. coins_awarded
// counts what the referrer has received so far; it carries no default, so a
// document written before the two-stage payout reads as 0 here and is skipped
// by the payout query instead of being paid a second time.
type ReferralEvent struct {
	mongox.Model     `bson:",inline"`
	ReferrerPlayerID string `bson:"referrer_player_id"`
	RefereePlayerID  string `bson:"referee_player_id"`
	ReferrerReward   int64  `bson:"referrer_reward"`
	RefereeReward    int64  `bson:"referee_reward"`
	CoinsAwarded     int64  `bson:"coins_awarded"`
	VerificationPaid bool   `bson:"verification_paid"`
	JoinPaid         bool   `bson:"join_paid"`
	RefereePaid      bool   `bson:"referee_paid"`
	// Checks and NextCheckAt pace the payout loop: an event whose player has not
	// moved yet is looked at once an hour instead of once a pass.
	Checks      int       `bson:"checks"`
	NextCheckAt time.Time `bson:"next_check_at"`
}
