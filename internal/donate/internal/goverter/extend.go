package goverter

import (
	"time"

	donatev1 "github.com/lasthearth/vsservice/gen/donate/v1"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
	"google.golang.org/protobuf/types/known/timestamppb"
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

func PrivilegeModelToProto(p model.Privilege) *donatev1.Privilege {
	return &donatev1.Privilege{
		Text: p.Text,
		Icon: p.Icon,
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

func TimePtrToTimestamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

func TimestampToTimePtr(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	t := ts.AsTime()
	return &t
}
