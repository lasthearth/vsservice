package model

import (
	"testing"
	"time"
)

func TestNewShopItem(t *testing.T) {
	item := NewShopItem("skin_default", "Skin", "Cool skin", "https://cdn/skin.png", 500)

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
	if item.Type != ItemTypeItem {
		t.Errorf("Type = %v, want item", item.Type)
	}
}

func TestShopItem_Validate(t *testing.T) {
	tests := []struct {
		name    string
		build   func() *ShopItem
		wantErr bool
	}{
		{
			"valid item",
			func() *ShopItem { return NewShopItem("skin_a", "Skin", "desc", "url", 100) },
			false,
		},
		{
			"empty code",
			func() *ShopItem { return NewShopItem("", "Skin", "desc", "url", 100) },
			true,
		},
		{
			"empty name",
			func() *ShopItem { return NewShopItem("skin_a", "", "desc", "url", 100) },
			true,
		},
		{
			"zero price",
			func() *ShopItem { return NewShopItem("skin_a", "Skin", "desc", "url", 0) },
			true,
		},
		{
			"negative price",
			func() *ShopItem { return NewShopItem("skin_a", "Skin", "desc", "url", -1) },
			true,
		},
		{
			"kit with empty entries",
			func() *ShopItem {
				return NewKitShopItem("kit_a", "Kit", "desc", "url", 100, []KitEntry{})
			},
			true,
		},
		{
			"kit with valid entries",
			func() *ShopItem {
				return NewKitShopItem("kit_a", "Kit", "desc", "url", 100, []KitEntry{
					{Name: "Sword", Quantity: 1},
				})
			},
			false,
		},
		{
			"kit entry quantity zero",
			func() *ShopItem {
				return NewKitShopItem("kit_a", "Kit", "desc", "url", 100, []KitEntry{
					{Name: "Sword", Quantity: 0},
				})
			},
			true,
		},
		{
			"discount percent 101",
			func() *ShopItem {
				item := NewShopItem("skin_a", "Skin", "desc", "url", 100)
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
	item := NewShopItem("old_code", "Old", "old desc", "old-url", 100)
	item.Apply(ShopItemUpdate{
		Code:        "new_code",
		Name:        "New",
		Description: "new desc",
		ImageURL:    "new-url",
		Price:       200,
		IsAvailable: false,
		Type:        ItemTypeItem,
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
			item := NewShopItem("code", "Name", "desc", "url", tc.price)
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
	item := NewShopItem("code", "Name", "desc", "url", 100)

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
	item := NewShopItem("code", "Name", "desc", "url", 100)
	_ = item.SetDiscount(50)
	item.ClearDiscount()

	if item.HasDiscount {
		t.Error("HasDiscount should be false after ClearDiscount")
	}
	if item.DiscountPercent != 0 {
		t.Errorf("DiscountPercent = %v, want 0", item.DiscountPercent)
	}
}

func TestShopItem_DiscountActive(t *testing.T) {
	base := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	past := base.Add(-time.Hour)
	future := base.Add(time.Hour)

	makeItem := func(percent int32, start, end *time.Time) *ShopItem {
		item := NewShopItem("code", "Name", "desc", "url", 100)
		if percent > 0 {
			_ = item.SetDiscount(percent)
		}
		item.SetDiscountWindow(start, end)
		return item
	}

	tests := []struct {
		name string
		item *ShopItem
		now  time.Time
		want bool
	}{
		{"nil window, discount active", makeItem(50, nil, nil), base, true},
		{"now before start, inactive", makeItem(50, &future, nil), base, false},
		{"now in window", makeItem(50, &past, &future), base, true},
		{"now == end, exclusive, inactive", makeItem(50, &past, &base), base, false},
		{"now after end, inactive", makeItem(50, &past, &past), base, false},
		{"HasDiscount=false, window set, inactive", func() *ShopItem {
			item := NewShopItem("code", "Name", "desc", "url", 100)
			item.SetDiscountWindow(&past, &future)
			return item
		}(), base, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.item.DiscountActive(tc.now)
			if got != tc.want {
				t.Errorf("DiscountActive() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestShopItem_EffectivePriceAt(t *testing.T) {
	base := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	past := base.Add(-time.Hour)
	future := base.Add(time.Hour)

	tests := []struct {
		name    string
		price   int64
		percent int32
		start   *time.Time
		end     *time.Time
		now     time.Time
		want    int64
	}{
		{"in window, 50%", 100, 50, &past, &future, base, 50},
		{"before window, full price", 100, 50, &future, nil, base, 100},
		{"after window (now==end), full price", 100, 50, &past, &base, base, 100},
		{"nil window, 50%", 100, 50, nil, nil, base, 50},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			item := NewShopItem("code", "Name", "desc", "url", tc.price)
			_ = item.SetDiscount(tc.percent)
			item.SetDiscountWindow(tc.start, tc.end)
			got := item.EffectivePriceAt(tc.now)
			if got != tc.want {
				t.Errorf("EffectivePriceAt() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestShopItem_Validate_DiscountWindow(t *testing.T) {
	t.Run("end before start returns error", func(t *testing.T) {
		item := NewShopItem("code", "Name", "desc", "url", 100)
		start := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)
		end := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
		item.SetDiscountWindow(&start, &end)
		if err := item.Validate(); err == nil {
			t.Error("expected error for end <= start, got nil")
		}
	})

	t.Run("end == start returns error", func(t *testing.T) {
		item := NewShopItem("code", "Name", "desc", "url", 100)
		ts := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
		item.SetDiscountWindow(&ts, &ts)
		if err := item.Validate(); err == nil {
			t.Error("expected error for end == start, got nil")
		}
	})

	t.Run("end after start is valid", func(t *testing.T) {
		item := NewShopItem("code", "Name", "desc", "url", 100)
		start := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
		end := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)
		item.SetDiscountWindow(&start, &end)
		if err := item.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
