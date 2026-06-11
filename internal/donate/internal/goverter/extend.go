package goverter

import (
	donatev1 "github.com/lasthearth/vsservice/gen/donate/v1"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
)

func ItemTypeModelToProto(t model.ItemType) donatev1.ItemType {
	switch t {
	case model.ItemTypeKit:
		return donatev1.ItemType_ITEM_TYPE_KIT
	case model.ItemTypeItem:
		return donatev1.ItemType_ITEM_TYPE_ITEM
	default:
		return donatev1.ItemType_ITEM_TYPE_ITEM
	}
}

func KitEntryModelToProto(e model.KitEntry) *donatev1.KitEntry {
	return &donatev1.KitEntry{
		Name:        e.Name,
		Description: e.Description,
		ImageUrl:    e.ImageURL,
		Quantity:    e.Quantity,
	}
}

func PtrStringToString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func PurchaseStatusToString(s model.PurchaseStatus) string { return string(s) }

func TxTypeToString(t model.TxType) string { return string(t) }

func ShopItemEffectivePrice(s *model.ShopItem) int64 { return s.EffectivePrice() }
