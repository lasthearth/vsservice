package model

import (
	"testing"
)

func TestNewPurchase(t *testing.T) {
	p := NewPurchase("user-1", "Player1", "item-1", "Cool Skin", 400, 500, 20)

	if p.PlayerID != "user-1" {
		t.Errorf("PlayerID = %v, want user-1", p.PlayerID)
	}
	if p.ItemName != "Cool Skin" {
		t.Errorf("ItemName = %v, want Cool Skin", p.ItemName)
	}
	if p.PricePaid != 400 {
		t.Errorf("PricePaid = %v, want 400", p.PricePaid)
	}
	if p.BasePrice != 500 {
		t.Errorf("BasePrice = %v, want 500", p.BasePrice)
	}
	if p.DiscountPercent != 20 {
		t.Errorf("DiscountPercent = %v, want 20", p.DiscountPercent)
	}
	if p.Status != PurchaseStatusActive {
		t.Errorf("Status = %v, want active", p.Status)
	}
	if p.RefundedAt != nil {
		t.Errorf("RefundedAt should be nil on creation")
	}
}

func TestNewPurchase_NoDiscount(t *testing.T) {
	p := NewPurchase("user-1", "Player1", "item-1", "Cool Skin", 500, 500, 0)

	if p.PricePaid != 500 {
		t.Errorf("PricePaid = %v, want 500", p.PricePaid)
	}
	if p.BasePrice != 500 {
		t.Errorf("BasePrice = %v, want 500", p.BasePrice)
	}
	if p.DiscountPercent != 0 {
		t.Errorf("DiscountPercent = %v, want 0", p.DiscountPercent)
	}
}

func TestPurchase_Refund(t *testing.T) {
	tests := []struct {
		name    string
		build   func() *Purchase
		wantErr bool
	}{
		{
			"active purchase can be refunded",
			func() *Purchase { return NewPurchase("u", "p", "i", "item", 100, 100, 0) },
			false,
		},
		{
			"already refunded purchase returns error",
			func() *Purchase {
				p := NewPurchase("u", "p", "i", "item", 100, 100, 0)
				_ = p.Refund()
				return p
			},
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.build()
			err := p.Refund()
			if (err != nil) != tc.wantErr {
				t.Errorf("Refund() error = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil {
				if p.Status != PurchaseStatusRefunded {
					t.Errorf("Status = %v, want refunded", p.Status)
				}
				if p.RefundedAt == nil {
					t.Errorf("RefundedAt should be set after refund")
				}
			}
		})
	}
}

func TestPurchase_IsActive(t *testing.T) {
	tests := []struct {
		status PurchaseStatus
		want   bool
	}{
		{PurchaseStatusActive, true},
		{PurchaseStatusRefunded, false},
	}
	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			p := &Purchase{Status: tc.status}
			if got := p.IsActive(); got != tc.want {
				t.Errorf("IsActive() = %v, want %v", got, tc.want)
			}
		})
	}
}
