package model_test

import (
	"testing"

	"github.com/lasthearth/vsservice/internal/donate/internal/model"
)

func TestNewWallet(t *testing.T) {
	w := model.NewWallet("user-1", "Player1")

	if w.PlayerID != "user-1" {
		t.Errorf("PlayerID = %v, want user-1", w.PlayerID)
	}
	if w.PlayerName != "Player1" {
		t.Errorf("PlayerName = %v, want Player1", w.PlayerName)
	}
	if w.Coins != 0 {
		t.Errorf("Coins = %v, want 0", w.Coins)
	}
}

func TestWallet_Deposit(t *testing.T) {
	tests := []struct {
		name      string
		initial   int64
		amount    int64
		wantCoins int64
		wantErr   bool
	}{
		{"positive amount credits coins", 100, 50, 150, false},
		{"deposit from zero", 0, 1, 1, false},
		{"zero amount rejected", 100, 0, 100, true},
		{"negative amount rejected", 100, -10, 100, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := &model.Wallet{Coins: tc.initial}
			err := w.Deposit(tc.amount)
			if (err != nil) != tc.wantErr {
				t.Errorf("Deposit(%v) error = %v, wantErr %v", tc.amount, err, tc.wantErr)
			}
			if w.Coins != tc.wantCoins {
				t.Errorf("Coins = %v, want %v", w.Coins, tc.wantCoins)
			}
		})
	}
}

func TestWallet_Withdraw(t *testing.T) {
	tests := []struct {
		name      string
		initial   int64
		amount    int64
		wantCoins int64
		wantErr   bool
	}{
		{"sufficient funds", 100, 50, 50, false},
		{"exact balance", 100, 100, 0, false},
		{"insufficient funds", 50, 100, 50, true},
		{"zero amount rejected", 100, 0, 100, true},
		{"negative amount rejected", 100, -10, 100, true},
		{"withdraw from empty wallet", 0, 1, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := &model.Wallet{Coins: tc.initial}
			err := w.Withdraw(tc.amount)
			if (err != nil) != tc.wantErr {
				t.Errorf("Withdraw(%v) error = %v, wantErr %v", tc.amount, err, tc.wantErr)
			}
			if w.Coins != tc.wantCoins {
				t.Errorf("Coins = %v, want %v", w.Coins, tc.wantCoins)
			}
		})
	}
}
