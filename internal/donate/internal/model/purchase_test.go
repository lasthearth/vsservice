package model_test

import (
	"testing"

	"github.com/lasthearth/vsservice/internal/donate/internal/model"
)

func TestNewPurchase(t *testing.T) {
	p := model.NewPurchase("user-1", "Player1", "item-1", "Cool Skin", 500)

	if p.PlayerID != "user-1" {
		t.Errorf("PlayerID = %v, want user-1", p.PlayerID)
	}
	if p.ItemName != "Cool Skin" {
		t.Errorf("ItemName = %v, want Cool Skin", p.ItemName)
	}
	if p.PricePaid != 500 {
		t.Errorf("PricePaid = %v, want 500", p.PricePaid)
	}
	if p.Status != model.PurchaseStatusActive {
		t.Errorf("Status = %v, want active", p.Status)
	}
	if p.RefundedAt != nil {
		t.Errorf("RefundedAt should be nil on creation")
	}
}

func TestPurchase_Refund(t *testing.T) {
	tests := []struct {
		name    string
		build   func() *model.Purchase
		wantErr bool
	}{
		{
			"active purchase can be refunded",
			func() *model.Purchase { return model.NewPurchase("u", "p", "i", "item", 100) },
			false,
		},
		{
			"already refunded purchase returns error",
			func() *model.Purchase {
				p := model.NewPurchase("u", "p", "i", "item", 100)
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
				if p.Status != model.PurchaseStatusRefunded {
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
		status model.PurchaseStatus
		want   bool
	}{
		{model.PurchaseStatusActive, true},
		{model.PurchaseStatusRefunded, false},
	}
	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			p := &model.Purchase{Status: tc.status}
			if got := p.IsActive(); got != tc.want {
				t.Errorf("IsActive() = %v, want %v", got, tc.want)
			}
		})
	}
}
