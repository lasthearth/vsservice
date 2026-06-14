package model

import (
	"errors"
	"time"
)

// Wallet holds the coin balance for a single player.
type Wallet struct {
	Id         string
	PlayerID   string
	PlayerName string
	Coins      int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func NewWallet(playerID, playerName string) *Wallet {
	return &Wallet{
		PlayerID:   playerID,
		PlayerName: playerName,
		Coins:      0,
	}
}

// ReconstituteWallet rebuilds a Wallet from persisted state. Repository use only.
func ReconstituteWallet(id, playerID, playerName string, coins int64, createdAt, updatedAt time.Time) *Wallet {
	return &Wallet{
		Id:         id,
		PlayerID:   playerID,
		PlayerName: playerName,
		Coins:      coins,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}
}

// Touch records the wallet's last modification time.
func (w *Wallet) Touch(now time.Time) { w.UpdatedAt = now }

// Deposit adds amount to the wallet. Amount must be positive.
func (w *Wallet) Deposit(amount int64) error {
	if amount <= 0 {
		return errors.New("deposit amount must be positive")
	}
	w.Coins += amount
	return nil
}

// Withdraw deducts amount from the wallet. Returns an error if funds are insufficient.
func (w *Wallet) Withdraw(amount int64) error {
	if amount <= 0 {
		return errors.New("withdraw amount must be positive")
	}
	if w.Coins < amount {
		return errors.New("insufficient funds")
	}
	w.Coins -= amount
	return nil
}
