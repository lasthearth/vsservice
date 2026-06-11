package model_test

import (
	"testing"

	"github.com/lasthearth/vsservice/internal/donate/internal/model"
)

func TestNewShopItem(t *testing.T) {
	item := model.NewShopItem("skin_default", "Skin", "Cool skin", "https://cdn/skin.png", 500)

	if item.Code != "skin_default" {
		t.Errorf("Code = %v, want skin_default", item.Code)
	}
	if item.Name != "Skin" {
		t.Errorf("Name = %v, want Skin", item.Name)
	}
	if item.Price != 500 {
		t.Errorf("Price = %v, want 500", item.Price)
	}
	if !item.IsAvailable {
		t.Errorf("IsAvailable should be true on creation")
	}
	if item.Type != model.ItemTypeItem {
		t.Errorf("Type = %v, want item", item.Type)
	}
}

func TestShopItem_Validate(t *testing.T) {
	tests := []struct {
		name    string
		build   func() *model.ShopItem
		wantErr bool
	}{
		{
			"valid item",
			func() *model.ShopItem { return model.NewShopItem("skin_a", "Skin", "desc", "url", 100) },
			false,
		},
		{
			"empty code",
			func() *model.ShopItem { return model.NewShopItem("", "Skin", "desc", "url", 100) },
			true,
		},
		{
			"empty name",
			func() *model.ShopItem { return model.NewShopItem("skin_a", "", "desc", "url", 100) },
			true,
		},
		{
			"zero price",
			func() *model.ShopItem { return model.NewShopItem("skin_a", "Skin", "desc", "url", 0) },
			true,
		},
		{
			"negative price",
			func() *model.ShopItem { return model.NewShopItem("skin_a", "Skin", "desc", "url", -1) },
			true,
		},
		{
			"kit with empty entries",
			func() *model.ShopItem {
				return model.NewKitShopItem("kit_a", "Kit", "desc", "url", 100, []model.KitEntry{})
			},
			true,
		},
		{
			"kit with valid entries",
			func() *model.ShopItem {
				return model.NewKitShopItem("kit_a", "Kit", "desc", "url", 100, []model.KitEntry{
					{Name: "Sword", Quantity: 1},
				})
			},
			false,
		},
		{
			"kit entry quantity zero",
			func() *model.ShopItem {
				return model.NewKitShopItem("kit_a", "Kit", "desc", "url", 100, []model.KitEntry{
					{Name: "Sword", Quantity: 0},
				})
			},
			true,
		},
		{
			"discount percent 101",
			func() *model.ShopItem {
				item := model.NewShopItem("skin_a", "Skin", "desc", "url", 100)
				item.HasDiscount = true
				item.DiscountPercent = 101
				return item
			},
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.build().Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestShopItem_Apply(t *testing.T) {
	item := model.NewShopItem("old_code", "Old", "old desc", "old-url", 100)
	item.Apply(model.ShopItemUpdate{
		Code:        "new_code",
		Name:        "New",
		Description: "new desc",
		ImageURL:    "new-url",
		Price:       200,
		IsAvailable: false,
		Type:        model.ItemTypeItem,
	})

	if item.Code != "new_code" {
		t.Errorf("Code = %v, want new_code", item.Code)
	}
	if item.Name != "New" {
		t.Errorf("Name = %v, want New", item.Name)
	}
	if item.Description != "new desc" {
		t.Errorf("Description = %v, want new desc", item.Description)
	}
	if item.ImageURL != "new-url" {
		t.Errorf("ImageURL = %v, want new-url", item.ImageURL)
	}
	if item.Price != 200 {
		t.Errorf("Price = %v, want 200", item.Price)
	}
	if item.IsAvailable {
		t.Errorf("IsAvailable = true, want false")
	}
}

func TestShopItem_EffectivePrice(t *testing.T) {
	tests := []struct {
		name     string
		price    int64
		discount bool
		percent  int32
		want     int64
	}{
		{"no discount", 100, false, 0, 100},
		{"0 percent discount", 100, true, 0, 100},
		{"50 percent from 100", 100, true, 50, 50},
		{"100 percent clamp to 1", 100, true, 100, 1},
		{"price 1 with 100 percent", 1, true, 100, 1},
		{"10 percent from 200", 200, true, 10, 180},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			item := model.NewShopItem("code", "Name", "desc", "url", tc.price)
			if tc.discount {
				if err := item.SetDiscount(tc.percent); err != nil {
					t.Fatalf("SetDiscount failed: %v", err)
				}
			}
			got := item.EffectivePrice()
			if got != tc.want {
				t.Errorf("EffectivePrice() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestShopItem_SetDiscount(t *testing.T) {
	item := model.NewShopItem("code", "Name", "desc", "url", 100)

	if err := item.SetDiscount(0); err != nil {
		t.Errorf("SetDiscount(0) unexpected error: %v", err)
	}
	if !item.HasDiscount {
		t.Error("HasDiscount should be true after SetDiscount")
	}

	if err := item.SetDiscount(100); err != nil {
		t.Errorf("SetDiscount(100) unexpected error: %v", err)
	}

	if err := item.SetDiscount(101); err == nil {
		t.Error("SetDiscount(101) should return error")
	}

	if err := item.SetDiscount(-1); err == nil {
		t.Error("SetDiscount(-1) should return error")
	}
}

func TestShopItem_ClearDiscount(t *testing.T) {
	item := model.NewShopItem("code", "Name", "desc", "url", 100)
	_ = item.SetDiscount(50)
	item.ClearDiscount()

	if item.HasDiscount {
		t.Error("HasDiscount should be false after ClearDiscount")
	}
	if item.DiscountPercent != 0 {
		t.Errorf("DiscountPercent = %v, want 0", item.DiscountPercent)
	}
}
