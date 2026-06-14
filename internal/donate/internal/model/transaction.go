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

// ReconstituteTransaction rebuilds a Transaction from persisted state. Repository use only.
func ReconstituteTransaction(id, playerID string, amount int64, txType TxType, reason, purchaseID string, createdAt time.Time) *Transaction {
	return &Transaction{
		Id:         id,
		PlayerID:   playerID,
		Amount:     amount,
		Type:       txType,
		Reason:     reason,
		PurchaseID: purchaseID,
		CreatedAt:  createdAt,
	}
}

// AttachPurchase links this transaction to a purchase.
func (t *Transaction) AttachPurchase(purchaseID string) { t.PurchaseID = purchaseID }

// MarkCreated records the persisted identity and creation time.
func (t *Transaction) MarkCreated(id string, createdAt time.Time) {
	t.Id = id
	t.CreatedAt = createdAt
}
