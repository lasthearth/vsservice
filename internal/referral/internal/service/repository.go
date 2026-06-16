package service

import (
	"context"

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
}
