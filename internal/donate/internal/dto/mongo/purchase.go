package dto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
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
	IssuedAt     *time.Time `bson:"issued_at,omitempty"`
	IssuedBy     *string    `bson:"issued_by,omitempty"`
}

// Id satisfies pagination.Identifiable for cursor-based pagination.
func (p Purchase) Id() bson.ObjectID {
	return p.Model.Id
}
