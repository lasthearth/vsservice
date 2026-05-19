package model_test

import (
	"testing"

	"github.com/lasthearth/vsservice/internal/donate/internal/model"
)

func TestNewShopItem(t *testing.T) {
	item := model.NewShopItem("Skin", "Cool skin", "https://cdn/skin.png", 500)

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
			func() *model.ShopItem { return model.NewShopItem("Skin", "desc", "url", 100) },
			false,
		},
		{
			"empty name",
			func() *model.ShopItem { return model.NewShopItem("", "desc", "url", 100) },
			true,
		},
		{
			"zero price",
			func() *model.ShopItem { return model.NewShopItem("Skin", "desc", "url", 0) },
			true,
		},
		{
			"negative price",
			func() *model.ShopItem { return model.NewShopItem("Skin", "desc", "url", -1) },
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
	item := model.NewShopItem("Old", "old desc", "old-url", 100)
	item.Update("New", "new desc", "new-url", 200, false)

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
