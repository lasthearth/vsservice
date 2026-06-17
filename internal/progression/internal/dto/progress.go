package dto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type PurchasedNode struct {
	NodeId                string    `bson:"node_id"`
	PurchasedAt           time.Time `bson:"purchased_at"`
	PurchasedBySettlement string    `bson:"purchased_by_settlement"`
}

type TalentProgress struct {
	mongox.Model   `bson:",inline"`
	OwnerType      string          `bson:"owner_type"`
	SettlementId   bson.ObjectID   `bson:"settlement_id,omitempty"`
	PointId        bson.ObjectID   `bson:"point_id,omitempty"`
	Side           string          `bson:"side,omitempty"`
	TreeId         bson.ObjectID   `bson:"tree_id"`
	PurchasedNodes []PurchasedNode `bson:"purchased_nodes"`
}
