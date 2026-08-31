package goverter

import (
	kitdefv1 "github.com/lasthearth/vsservice/gen/kitdef/v1"
	"github.com/lasthearth/vsservice/internal/kitdef/internal/model"
)

// KitItemModelToProto converts a domain KitItem to its proto form. attr_snapshot
// is intentionally dropped: it is server-side only and has no wire field.
func KitItemModelToProto(i model.KitItem) *kitdefv1.KitItem {
	return &kitdefv1.KitItem{
		GameCode: i.GameCode,
		Type:     i.Type,
		Quantity: i.Quantity,
		ImageUrl: i.ImageURL,
	}
}
