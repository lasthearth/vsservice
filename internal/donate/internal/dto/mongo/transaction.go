package dto

import "github.com/lasthearth/vsservice/internal/pkg/mongox"

type Transaction struct {
	mongox.Model `bson:",inline"`
	PlayerID     string `bson:"player_id"`
	Amount       int64  `bson:"amount"`
	Type         string `bson:"type"`
	Reason       string `bson:"reason"`
	PurchaseID   string `bson:"purchase_id,omitempty"`
}
