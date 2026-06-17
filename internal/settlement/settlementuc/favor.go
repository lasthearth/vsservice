package settlementuc

import (
	"context"

	"github.com/lasthearth/vsservice/internal/settlement/model"
)

type FavorRepository interface {
	UpdateSettlement(
		ctx context.Context,
		id string,
		updateFn func(ctx context.Context, s *model.Settlement) (*model.Settlement, error),
	) (*model.Settlement, error)
	IsLeaderOfSettlement(ctx context.Context, settlementID, userID string) error
	CreateFavorLog(ctx context.Context, log model.ImperialFavorLog) error
}

type FavorOps struct {
	repo FavorRepository
}

func NewFavorOps(repo FavorRepository) *FavorOps {
	return &FavorOps{repo: repo}
}

// Deduct removes amount from a settlement's imperial favor balance and records a log entry.
func (f *FavorOps) Deduct(ctx context.Context, settlementID string, amount int64, reason, byPlayerID string) error {
	_, err := f.repo.UpdateSettlement(ctx, settlementID,
		func(_ context.Context, s *model.Settlement) (*model.Settlement, error) {
			return s, s.DeductFavor(amount)
		},
	)
	if err != nil {
		return err
	}
	_ = f.repo.CreateFavorLog(ctx, model.ImperialFavorLog{
		SettlementId: settlementID,
		AdminId:      byPlayerID,
		Amount:       -amount,
		Reason:       reason,
	})
	return nil
}

// Add increases amount for a settlement's imperial favor balance and records a log entry.
func (f *FavorOps) Add(ctx context.Context, settlementID string, amount int64, reason, byPlayerID string) error {
	_, err := f.repo.UpdateSettlement(ctx, settlementID,
		func(_ context.Context, s *model.Settlement) (*model.Settlement, error) {
			s.AddFavor(amount)
			return s, nil
		},
	)
	if err != nil {
		return err
	}
	_ = f.repo.CreateFavorLog(ctx, model.ImperialFavorLog{
		SettlementId: settlementID,
		AdminId:      byPlayerID,
		Amount:       amount,
		Reason:       reason,
	})
	return nil
}

// IsLeader checks that playerID is the leader of settlementID.
func (f *FavorOps) IsLeader(ctx context.Context, settlementID, playerID string) error {
	return f.repo.IsLeaderOfSettlement(ctx, settlementID, playerID)
}
