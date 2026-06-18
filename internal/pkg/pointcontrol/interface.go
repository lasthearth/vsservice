package pointcontrol

import "context"

// Reader retrieves the settlement currently controlling an imperial point.
// Implemented by internal/imperial-point Service, consumed by internal/progression Service.
type Reader interface {
	GetControllingSettlement(ctx context.Context, pointId string) (string, error)
}
