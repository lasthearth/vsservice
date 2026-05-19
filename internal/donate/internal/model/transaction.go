package model

import "time"

type TxType string

const (
	TxTypeCredit TxType = "credit"
	TxTypeDebit  TxType = "debit"
)

// Transaction is an immutable record of a coin credit or debit.
type Transaction struct {
	Id         string
	PlayerID   string
	Amount     int64
	Type       TxType
	Reason     string
	PurchaseID string
	CreatedAt  time.Time
}

func NewCreditTransaction(playerID string, amount int64, reason string) *Transaction {
	return &Transaction{
		PlayerID: playerID,
		Amount:   amount,
		Type:     TxTypeCredit,
		Reason:   reason,
	}
}

func NewDebitTransaction(playerID string, amount int64, reason string) *Transaction {
	return &Transaction{
		PlayerID: playerID,
		Amount:   amount,
		Type:     TxTypeDebit,
		Reason:   reason,
	}
}
