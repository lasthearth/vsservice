package dto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
)

type Purchase struct {
	mongox.Model `bson:",inline"`
	PlayerID     string     `bson:"player_id"`
	PlayerName   string     `bson:"player_name"`
	ItemID       string     `bson:"item_id"`
	ItemName     string     `bson:"item_name"`
	PricePaid    int64      `bson:"price_paid"`
	Status       string     `bson:"status"`
	RefundedAt   *time.Time `bson:"refunded_at,omitempty"`
}
