package pointcontrol

import "context"

// Reader retrieves the settlement currently controlling an imperial point.
// Implemented by internal/imperial-point Service, consumed by internal/progression Service.
type Reader interface {
	GetControllingSettlement(ctx context.Context, pointId string) (string, error)
}

// Rollbacker rolls back the last purchased progression node for a point+side+tree.
// Implemented by internal/progression Service, consumed by internal/imperial-point Service.
type Rollbacker interface {
	RollbackLastPointNode(ctx context.Context, pointId, side, treeId string) error
}
