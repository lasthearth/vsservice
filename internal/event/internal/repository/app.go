package repository

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	collName          = "events"
	attendeesCollName = "event_attendees"
)

type Opts struct {
	fx.In
	Logger logger.Logger
	Db     *mongo.Database
	Mapper Mapper
}

type Repository struct {
	logger    logger.Logger
	coll      *mongo.Collection
	attendees *mongo.Collection
	mapper    Mapper
}

func New(opts Opts) *Repository {
	l := opts.Logger.WithComponent("repository")
	coll := opts.Db.Collection(collName)
	attendees := opts.Db.Collection(attendeesCollName)
	setupIndexes(l, coll)
	setupAttendeeIndexes(l, attendees)

	return &Repository{
		logger:    l,
		coll:      coll,
		attendees: attendees,
		mapper:    opts.Mapper,
	}
}

// eventIndexes are the indexes the events collection needs.
//
// Both list sorts end in _id, so both indexes must carry it too. A sort of
// {starts_at, _id} is not served by an index on {starts_at} alone: MongoDB does
// not treat _id as an implicit suffix of a secondary index, so the planner falls
// back to a blocking in-memory sort and undoes the point of the index.
// index_covers_sort_test.go pins the agreement, because the mismatch is silent.
//
// {starts_at, _id} serves ListUpcoming, which filters on until but returns
// starts_at order. A range on a leading until field cannot yield that order, so
// the planner scans starts_at in order, applies until per document, and lets the
// limit stop the scan early.
//
// These supersede the single-field {until: 1} index the collection may already
// carry; that one is now redundant and can be dropped by hand with
// db.events.dropIndex("until_1").
var eventIndexes = []mongo.IndexModel{
	// Serves ListPast: a range on until, returned in reverse until order.
	{Keys: bson.D{{Key: "until", Value: 1}, {Key: "_id", Value: 1}}},
	// Serves ListUpcoming.
	{Keys: bson.D{{Key: "starts_at", Value: 1}, {Key: "_id", Value: 1}}},
}

func setupIndexes(log logger.Logger, coll *mongo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := coll.Indexes().CreateMany(ctx, eventIndexes); err != nil {
		log.Error("failed to create indexes", zap.String("collection", collName), zap.Error(err))
	}
}

func setupAttendeeIndexes(log logger.Logger, coll *mongo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	models := []mongo.IndexModel{
		// One sign-up per player and event; also serves the per-event lists,
		// which filter on event_id.
		{
			Keys:    bson.D{{Key: "event_id", Value: 1}, {Key: "user_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		// The per-event lists in sign-up order.
		{Keys: bson.D{{Key: "event_id", Value: 1}, {Key: "created_at", Value: 1}}},
		// "My events".
		{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}}},
	}

	if _, err := coll.Indexes().CreateMany(ctx, models); err != nil {
		log.Error("failed to create indexes", zap.String("collection", attendeesCollName), zap.Error(err))
	}
}
