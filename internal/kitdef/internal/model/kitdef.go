package model

import "time"

// Category values for a captured kit. "kit" is the default: a kit whose
// category was never set (or was captured before the field existed) decodes
// as an empty string and reads as KitCategoryKit.
const (
	KitCategoryKit       = "kit"
	KitCategoryPrivilege = "privilege"
	KitCategorySpecial   = "special"
)

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
// Mongo-direct, plus the category chosen at capture); vsservice owns the
// display metadata (title, category). vsservice never inserts a KitDef, so
// there is no New* constructor — instances only ever come back from the
// repository via ReconstituteKitDef.
type KitDef struct {
	Code      string
	Title     string
	Category  string
	Items     []KitItem
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ReconstituteKitDef rebuilds a KitDef from persisted state. Repository use only.
func ReconstituteKitDef(code, title, category string, items []KitItem, createdAt, updatedAt time.Time) *KitDef {
	return &KitDef{
		Code:      code,
		Title:     title,
		Category:  category,
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

// SetCategory sets the display category. The value must be one of the
// KitCategory* constants; anything else (including the empty string) is
// rejected — the service layer decides "empty request field means unchanged",
// the model never stores an unknown category.
func (k *KitDef) SetCategory(category string) error {
	if !IsValidCategory(category) {
		return ErrUnknownCategory
	}
	k.Category = category
	return nil
}

// RemoveItemAt removes the item at position index, bounds-checked. Refuses
// when the removal would leave the kit empty: an empty kit is treated as
// missing everywhere downstream (a claimless kit-mail must never exist), so
// the last item is not removable — re-capture or delete the kit instead.
func (k *KitDef) RemoveItemAt(index int) error {
	if index < 0 || index >= len(k.Items) {
		return ErrItemIndex
	}
	if len(k.Items) <= 1 {
		return ErrLastItem
	}
	k.Items = append(k.Items[:index], k.Items[index+1:]...)
	return nil
}

// IsValidCategory reports whether category is one of the three KitCategory*
// constants.
func IsValidCategory(category string) bool {
	switch category {
	case KitCategoryKit, KitCategoryPrivilege, KitCategorySpecial:
		return true
	default:
		return false
	}
}
