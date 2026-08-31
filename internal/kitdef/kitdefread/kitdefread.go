// Package kitdefread is the kitdef domain's PUBLIC read port: the primitive-
// typed surface other domains (e.g. mail) use to read a captured kit's contents
// without importing kitdef internals. kitdef never imports the caller and the
// caller never imports kitdef/internal — the seam stays acyclic.
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

// KitDef is a captured kit's read-model: its code and its contents.
type KitDef struct {
	Code  string
	Items []KitItem
}

// Reader returns a captured kit by code. Implemented by the kitdef service and
// bound via fx.As in internal/kitdef/fx.go. Returns kitdef's ierror.ErrNotFound
// (a *pkgerr.DomainError with codes.NotFound) when the kit was never captured.
type Reader interface {
	GetKit(ctx context.Context, code string) (*KitDef, error)
}
