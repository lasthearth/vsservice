package serverinfo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// buildStaleOnlineFilter is the sweeper's whole query, kept free of the database
// so its two branches can be read and tested on their own.
//
// total_online is the guard: a document already at zero is not rewritten, so an
// idle pass touches nothing and the game's next 5-second write is never raced by
// a redundant $set. The $or is the staleness test, and both of its branches are
// load-bearing. The $exists branch is the rollout one: until the game server
// writes heartbeat_at for the first time, the singleton document has no such
// field, and a filter that only compared it would leave the online count of a
// long-dead server on the site forever.
func buildStaleOnlineFilter(cutoffUnixMilli int64) bson.M {
	return bson.M{
		"total_online": bson.M{"$ne": 0},
		"$or": bson.A{
			bson.M{"heartbeat_at": bson.M{"$lt": cutoffUnixMilli}},
			bson.M{"heartbeat_at": bson.M{"$exists": false}},
		},
	}
}

// buildStaleOnlineUpdate is the write the filter is applied with. world_time is
// deliberately left alone: the last reported time of day is more useful to the
// site than an empty one, and it goes stale on its own terms.
func buildStaleOnlineUpdate() bson.M {
	return bson.M{"$set": bson.M{"total_online": 0}}
}

// ClearStaleOnline zeroes total_online on documents whose heartbeat is older
// than cutoffUnixMilli, or that carry no heartbeat at all, and returns how many
// documents changed.
func (r *Repository) ClearStaleOnline(ctx context.Context, cutoffUnixMilli int64) (int64, error) {
	res, err := r.coll.UpdateMany(
		ctx,
		buildStaleOnlineFilter(cutoffUnixMilli),
		buildStaleOnlineUpdate(),
	)
	if err != nil {
		return 0, err
	}

	return res.ModifiedCount, nil
}
