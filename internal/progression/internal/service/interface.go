package service

import (
	"context"

	"github.com/lasthearth/vsservice/internal/progression/internal/model"
)

// ProgressionRepository is the data access interface consumed by Service.
// The concrete implementation lives in internal/progression/internal/repository/.
type ProgressionRepository interface {
	// Trees
	CreateTree(ctx context.Context, tree model.TalentTree) (*model.TalentTree, error)
	UpdateTree(ctx context.Context, tree model.TalentTree) (*model.TalentTree, error)
	GetTree(ctx context.Context, id string) (*model.TalentTree, error)
	ListTrees(ctx context.Context) ([]model.TalentTree, error)

	// Presets
	CreatePreset(ctx context.Context, preset model.TalentPreset) (*model.TalentPreset, error)
	UpdatePreset(ctx context.Context, preset model.TalentPreset) (*model.TalentPreset, error)
	GetPreset(ctx context.Context, id string) (*model.TalentPreset, error)
	ListPresets(ctx context.Context) ([]model.TalentPreset, error)

	// Progress
	GetOrCreateProgress(ctx context.Context, ownerType, settlementId, pointId, side, treeId string) (*model.TalentProgress, error)
	SaveProgress(ctx context.Context, progress model.TalentProgress) error
}

// FavorDeductor deducts imperial favor from a settlement.
// Implemented by settlementuc.FavorOps, injected via fx.
type FavorDeductor interface {
	Deduct(ctx context.Context, settlementID string, amount int64, reason, byPlayerID string) error
	IsLeader(ctx context.Context, settlementID, playerID string) error
}
