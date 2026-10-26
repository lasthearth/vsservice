package service

import (
	"context"
)

// KitItemView is one item of a captured kit as donate consumes it: what the
// витрина of a kit shop entry is built from — the game code, the stack size
// and the game-rendered icon uploaded at capture.
type KitItemView struct {
	GameCode string
	Quantity int32
	ImageURL string
}

// KitSummary is a captured kit's category and contents — everything donate
// needs from a kit besides its code.
type KitSummary struct {
	Category string
	Items    []KitItemView
}

// KitLookup is donate's consumer-side port over kitdef's public read port:
// the captured-kit catalog donate needs — to refuse a shop entry whose code
// was never captured in game, and to join a kit's category and витрина onto
// the shop list. The adapter over kitdefread.Reader lives in internal/donate
// (the composition seam); donate never imports kitdef internals.
type KitLookup interface {
	// Get returns the summary of the captured kit named by code. Returns
	// kitdef's NotFound domain error when the kit was never captured — that
	// is the "code not captured" refusal donate answers with.
	Get(ctx context.Context, code string) (KitSummary, error)

	// Catalog returns code -> summary for every captured kit in one call:
	// the ListShopItems join reads the catalog once instead of one lookup
	// per entry.
	Catalog(ctx context.Context) (map[string]KitSummary, error)
}
