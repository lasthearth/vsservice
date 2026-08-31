package model

import "time"

// KitItem is one entry of a kit's contents. game_code + type identify the asset
// (VintageAPI resolves it); attr_snapshot is a base64 TreeAttribute snapshot
// (empty = plain stack) that stays server-side — it is on the internal read
// path but never crosses the ListKits wire.
type KitItem struct {
	GameCode     string
	Type         string // "item" or "block"
	Quantity     int32
	AttrSnapshot string // base64 TreeAttribute; "" when empty. OPAQUE — never parse.
	ImageURL     string
}

// KitDef is a captured kit. The game owns code/items/created_at (written
// Mongo-direct); vsservice owns only the display title. vsservice never inserts
// a KitDef, so there is no New* constructor — instances only ever come back
// from the repository via ReconstituteKitDef.
type KitDef struct {
	Code      string
	Title     string
	Items     []KitItem
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ReconstituteKitDef rebuilds a KitDef from persisted state. Repository use only.
func ReconstituteKitDef(code, title string, items []KitItem, createdAt, updatedAt time.Time) *KitDef {
	return &KitDef{
		Code:      code,
		Title:     title,
		Items:     items,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
}

// Rename sets the display title. Returns an error on an empty title so the
// invariant (a kit always has a non-empty title once metadata is written) lives
// on the model.
func (k *KitDef) Rename(title string) error {
	if title == "" {
		return ErrEmptyTitle
	}
	k.Title = title
	return nil
}
