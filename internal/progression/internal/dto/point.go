package dto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type PointControl struct {
	Side            string        `bson:"side"`
	SettlementId    bson.ObjectID `bson:"settlement_id"`
	ControlledSince time.Time     `bson:"controlled_since"`
}

type ImperialPoint struct {
	mongox.Model  `bson:",inline"`
	Name          string        `bson:"name"`
	Description   string        `bson:"description"`
	BiRatePerHour int64         `bson:"bi_rate_per_hour"`
	TreeId        bson.ObjectID `bson:"tree_id,omitempty"`
	Control       *PointControl `bson:"control,omitempty"`
}
