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

func setupIndexes(log logger.Logger, coll *mongo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	models := []mongo.IndexModel{
		// Serves ListPast: a range on until, returned in until order.
		{Keys: bson.D{{Key: "until", Value: 1}}},
		// Serves ListUpcoming, which filters on until but returns starts_at
		// order. A range on the leading until field cannot yield that order, so
		// without this index the planner adds a blocking in-memory sort over
		// every upcoming event on a public endpoint. Scanning starts_at in order
		// and filtering until per document keeps the sort indexed and lets the
		// limit stop the scan early.
		{Keys: bson.D{{Key: "starts_at", Value: 1}}},
	}

	if _, err := coll.Indexes().CreateMany(ctx, models); err != nil {
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
