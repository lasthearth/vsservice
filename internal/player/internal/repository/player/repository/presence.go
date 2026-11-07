package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// buildStalePresenceFilter is the sweeper's whole query, kept free of the
// database so its two branches can be read and tested on their own.
//
// is_online is the guard: it is also the partial index's filter expression, so
// the pass only ever looks at the dozens of documents that claim to be online
// instead of the whole collection. The $or is the staleness test, and both of its
// branches are load-bearing. The $exists branch is the rollout one: a document
// that went online before the game started writing heartbeats carries is_online
// and no last_heartbeat at all, and a filter that only compared the heartbeat
// would leave that player online forever.
func buildStalePresenceFilter(cutoffUnixMilli int64) bson.M {
	return bson.M{
		"is_online": true,
		"$or": bson.A{
			bson.M{"last_heartbeat": bson.M{"$lt": cutoffUnixMilli}},
			bson.M{"last_heartbeat": bson.M{"$exists": false}},
		},
	}
}

// buildStalePresenceUpdate is the write the filter is applied with. Only the
// flag is touched: last_heartbeat stays as the game left it, so the roster's
// "last seen" and the next sweep's cutoff are both still readable from it.
func buildStalePresenceUpdate() bson.M {
	return bson.M{"$set": bson.M{"is_online": false}}
}

// ClearStalePresence takes offline every player whose heartbeat is older than
// cutoffUnixMilli, or who carries no heartbeat at all, and returns how many
// documents changed.
func (r *Repository) ClearStalePresence(ctx context.Context, cutoffUnixMilli int64) (int64, error) {
	res, err := r.coll.UpdateMany(
		ctx,
		buildStalePresenceFilter(cutoffUnixMilli),
		buildStalePresenceUpdate(),
	)
	if err != nil {
		return 0, err
	}

	return res.ModifiedCount, nil
}
