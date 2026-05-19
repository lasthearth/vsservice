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
