package model

import "time"

type PurchaseStatus string

const (
	PurchaseStatusActive   PurchaseStatus = "active"
	PurchaseStatusRefunded PurchaseStatus = "refunded"
)

// Purchase records a player's completed shop transaction.
type Purchase struct {
	Id         string
	PlayerID   string
	PlayerName string
	ItemID     string
	ItemName   string
	PricePaid  int64
	Status     PurchaseStatus
	CreatedAt  time.Time
	RefundedAt *time.Time
}

func NewPurchase(playerID, playerName, itemID, itemName string, price int64) *Purchase {
	return &Purchase{
		PlayerID:   playerID,
		PlayerName: playerName,
		ItemID:     itemID,
		ItemName:   itemName,
		PricePaid:  price,
		Status:     PurchaseStatusActive,
	}
}

// Refund marks the purchase as refunded. Returns an error if already refunded.
func (p *Purchase) Refund() error {
	if p.Status == PurchaseStatusRefunded {
		return errAlreadyRefunded
	}
	now := time.Now()
	p.Status = PurchaseStatusRefunded
	p.RefundedAt = &now
	return nil
}

func (p *Purchase) IsActive() bool {
	return p.Status == PurchaseStatusActive
}
