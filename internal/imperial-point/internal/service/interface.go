package service

import (
	"context"

	"github.com/lasthearth/vsservice/internal/imperial-point/internal/model"
)

// ImperialPointRepository is the data access interface consumed by Service.
type ImperialPointRepository interface {
	CreatePoint(ctx context.Context, point model.ImperialPoint) (*model.ImperialPoint, error)
	UpdatePoint(ctx context.Context, point model.ImperialPoint) (*model.ImperialPoint, error)
	GetPoint(ctx context.Context, id string) (*model.ImperialPoint, error)
	ListPoints(ctx context.Context) ([]model.ImperialPoint, error)
	SaveControl(ctx context.Context, pointId string, control *model.PointControl) error
}

// ProgressionRollbacker rolls back the last purchased node for a point+side+tree.
// Implemented by internal/progression Service, injected via fx.
// Defined here as interface; concrete type is in internal/pkg/pointcontrol (shared).
type ProgressionRollbacker interface {
	RollbackLastPointNode(ctx context.Context, pointId, side, treeId string) error
}
