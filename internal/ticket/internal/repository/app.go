package repository

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	ticketColl = "tickets"
	replyColl  = "ticket_saved_replies"
)

type Opts struct {
	fx.In
	Logger logger.Logger
	Db     *mongo.Database
	Mapper Mapper
}

type Repository struct {
	logger  logger.Logger
	tickets *mongo.Collection
	replies *mongo.Collection
	mapper  Mapper
}

func New(opts Opts) *Repository {
	l := opts.Logger.WithComponent("repository")
	tickets := opts.Db.Collection(ticketColl)
	setupIndexes(l, tickets, ticketIndexes)

	return &Repository{
		logger:  l,
		tickets: tickets,
		replies: opts.Db.Collection(replyColl),
		mapper:  opts.Mapper,
	}
}

// ticketIndexes are the indexes the tickets collection needs. Each list index
// ends with the list's sort keys, {<time>: -1, _id: -1}, so MongoDB walks the
// index instead of sorting in memory.
//
// Created one at a time: CreateMany is all-or-nothing, so one conflicting spec
// would lose the others too.
var ticketIndexes = []mongo.IndexModel{
	// A player's list, and their active-ticket count (author_id prefix).
	{Keys: bson.D{{Key: "author_id", Value: 1}, {Key: "last_public_at", Value: -1}, {Key: "_id", Value: -1}}},
	// The staff inbox, unfiltered or by author.
	{Keys: bson.D{{Key: "last_message_at", Value: -1}, {Key: "_id", Value: -1}}},
	// The staff inbox by status, and the open counters.
	{Keys: bson.D{{Key: "status", Value: 1}, {Key: "last_message_at", Value: -1}, {Key: "_id", Value: -1}}},
	// Auto-close: answered tickets by when they were answered.
	{Keys: bson.D{{Key: "status", Value: 1}, {Key: "answered_at", Value: 1}}},
}

func setupIndexes(log logger.Logger, coll *mongo.Collection, models []mongo.IndexModel) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, model := range models {
		if _, err := coll.Indexes().CreateOne(ctx, model); err != nil {
			log.Error("failed to create index", zap.String("collection", coll.Name()), zap.Error(err))
		}
	}
}
