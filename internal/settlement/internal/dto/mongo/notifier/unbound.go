package notifierdto

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// UnboundNotifierCollection is the collection that holds the heralds standing
// with no settlement behind them.
const UnboundNotifierCollection = "unbound_notifiers"

// Unbound is a document of the unbound_notifiers collection: a herald block
// that stands with no settlement bound to it. The game server (VintageAPI) owns
// the writes, one row per block position; vsservice only reads it, so the
// document carries no mongox.Model envelope.
//
// These are the heralds the bound collection cannot hold: a manual tier
// (khutor, abandoned, normal, mine), a block an admin or a creative player
// placed, one an admin unbound with the block left standing, and an automated
// tier whose placer had no settlement to bind it to. A bound herald lives in
// settlement_notifiers instead, never in both, so the map reads the two
// collections and draws one pin per row.
type Unbound struct {
	Id       bson.ObjectID `bson:"_id,omitempty"`
	Position Position      `bson:"position"`
	// Variant is the block's tier variant, e.g. "khutor" or "camp". It tells the
	// pins apart on a map; the site does not serve it yet.
	Variant  string    `bson:"variant"`
	PlacedBy string    `bson:"placed_by"`
	PlacedAt time.Time `bson:"placed_at"`
}
