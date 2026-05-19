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

func TestShopItem_Update(t *testing.T) {
	item := model.NewShopItem("old_code", "Old", "old desc", "old-url", 100)
	item.Update("new_code", "New", "new desc", "new-url", 200, false)

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
