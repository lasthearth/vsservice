package service

import (
	"context"
)

// KitLookup is donate's consumer-side port over kitdef's public read port:
// the captured-kit catalog donate needs — to refuse a shop entry whose code
// was never captured in game, and to join a kit's category onto the shop
// list. The adapter over kitdefread.Reader lives in internal/donate (the
// composition seam); donate never imports kitdef internals.
type KitLookup interface {
	// GetCategory returns the category ("kit" | "privilege" | "special") of
	// the captured kit named by code. Returns kitdef's NotFound domain error
	// when the kit was never captured — that is the "code not captured"
	// refusal donate answers with.
	GetCategory(ctx context.Context, code string) (string, error)

	// Categories returns code -> category for every captured kit in one
	// call: the ListShopItems join reads the catalog once instead of one
	// lookup per entry.
	Categories(ctx context.Context) (map[string]string, error)
}
