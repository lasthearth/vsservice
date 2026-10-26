// Package kitdefread is the kitdef domain's PUBLIC read port: the primitive-
// typed surface other domains (e.g. mail, donate) use to read a captured kit's
// contents and metadata without importing kitdef internals. kitdef never
// imports the caller and the caller never imports kitdef/internal — the seam
// stays acyclic.
package kitdefread

import "context"

// KitItem is one entry of a kit's contents. game_code + type identify the asset
// (VintageAPI resolves it at claim); attr_snapshot is an opaque base64
// TreeAttribute snapshot ("" = plain stack) carried through server-side.
type KitItem struct {
	GameCode     string
	Type         string
	Quantity     int32
	AttrSnapshot string
}

// KitDef is a captured kit's read-model: its code, its contents and its
// category ("kit" | "privilege" | "special"; reconstitution defaults an unset
// category to "kit").
type KitDef struct {
	Code     string
	Category string
	Items    []KitItem
}

// Reader reads captured kits. Implemented by the kitdef service and bound via
// fx.As in internal/kitdef/fx.go. GetKit returns kitdef's
// ierror.ErrNotFound (a *pkgerr.DomainError with codes.NotFound) when the kit
// was never captured.
type Reader interface {
	// GetKit returns one captured kit by code.
	GetKit(ctx context.Context, code string) (*KitDef, error)

	// ListKits returns every captured kit in one call. Consumers that need
	// category-by-code (donate's shop list join) read the whole catalog once
	// instead of per-code lookups.
	ListKits(ctx context.Context) ([]*KitDef, error)
}
