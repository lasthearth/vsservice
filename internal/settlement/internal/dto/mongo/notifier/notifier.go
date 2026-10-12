package notifierdto

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Notifier is a document of the settlement_notifiers collection. The game
// server (VintageAPI) owns the writes: it inserts the document when a notifier
// block is placed and deletes it when the block is broken. vsservice only reads
// it, so the document carries no mongox.Model envelope.
type Notifier struct {
	Id           bson.ObjectID `bson:"_id,omitempty"`
	SettlementId string        `bson:"settlement_id"`
	Position     Position      `bson:"position"`
	PlacedBy     string        `bson:"placed_by"`
	PlacedAt     time.Time     `bson:"placed_at"`
}

// Position is a block position in the game world.
type Position struct {
	X int `bson:"x"`
	Y int `bson:"y"`
	Z int `bson:"z"`
}
