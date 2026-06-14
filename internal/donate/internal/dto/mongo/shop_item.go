package dto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
)

type KitEntryDTO struct {
	Name        string `bson:"name"`
	Description string `bson:"description"`
	ImageURL    string `bson:"image_url"`
	Quantity    int32  `bson:"quantity"`
}

type PrivilegeDTO struct {
	Text string `bson:"text"`
	Icon string `bson:"icon"`
}

type ShopItem struct {
	mongox.Model     `bson:",inline"`
	Code             string         `bson:"code"`
	Name             string         `bson:"name"`
	Description      string         `bson:"description"`
	ImageURL         string         `bson:"image_url"`
	Price            int64          `bson:"price"`
	IsAvailable      bool           `bson:"is_available"`
	Type             string         `bson:"type"`
	Entries          []KitEntryDTO  `bson:"entries,omitempty"`
	HasDiscount      bool           `bson:"has_discount"`
	DiscountPercent  int32          `bson:"discount_percent"`
	Privileges       []PrivilegeDTO `bson:"privileges,omitempty"`
	DiscountStartsAt *time.Time     `bson:"discount_starts_at,omitempty"`
	DiscountEndsAt   *time.Time     `bson:"discount_ends_at,omitempty"`
}
