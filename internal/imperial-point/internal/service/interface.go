package service

import (
	"context"

	"github.com/lasthearth/vsservice/internal/imperial-point/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/pointcontrol"
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
type ProgressionRollbacker = pointcontrol.Rollbacker
