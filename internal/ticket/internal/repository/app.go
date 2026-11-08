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
	replies := opts.Db.Collection(replyColl)
	setupIndexes(l, tickets, ticketIndexes)
	setupIndexes(l, replies, replyIndexes)

	return &Repository{
		logger:  l,
		tickets: tickets,
		replies: replies,
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
	// A player's list, and their active-ticket and unread counts (author_id
	// prefix).
	{Keys: bson.D{{Key: "author_id", Value: 1}, {Key: "last_public_at", Value: -1}, {Key: "_id", Value: -1}}},
	// The staff inbox with no filter at all.
	{Keys: bson.D{{Key: "last_message_at", Value: -1}, {Key: "_id", Value: -1}}},
	// The staff inbox by status, and the open counters.
	{Keys: bson.D{{Key: "status", Value: 1}, {Key: "last_message_at", Value: -1}, {Key: "_id", Value: -1}}},
	// The staff inbox narrowed to one player.
	{Keys: bson.D{{Key: "author_id", Value: 1}, {Key: "last_message_at", Value: -1}, {Key: "_id", Value: -1}}},
	// "Taken by me" and "taken by nobody" in the staff inbox. Without an
	// assignee_id index the planner sorts by last_message_at and fetches every
	// document to test the residual filter.
	{Keys: bson.D{{Key: "assignee_id", Value: 1}, {Key: "last_message_at", Value: -1}, {Key: "_id", Value: -1}}},
	{Keys: bson.D{{Key: "status", Value: 1}, {Key: "assignee_id", Value: 1}, {Key: "last_message_at", Value: -1}, {Key: "_id", Value: -1}}},
	// Auto-close: answered tickets by when they were answered.
	{Keys: bson.D{{Key: "status", Value: 1}, {Key: "answered_at", Value: 1}}},
	// The staff unread badge. StaffCounters matches on
	// {status: open} OR {staff_unread: true}; a top-level $or only uses
	// indexes when every clause has one, so the unread side needs its own.
	// Partial: the flag is false on almost every document.
	{
		Keys:    bson.D{{Key: "staff_unread", Value: 1}},
		Options: options.Index().SetPartialFilterExpression(bson.M{"staff_unread": true}),
	},
}

// replyIndexes back the saved-reply list, which sorts by title.
var replyIndexes = []mongo.IndexModel{
	{Keys: bson.D{{Key: "title", Value: 1}}},
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
