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

const collName = "notifications"

// retention is how long a notification stays in the collection. No RPC deletes
// one, so the TTL index is the only bound on a collection any producer can write
// into another player's inbox.
const retention = 90 * 24 * time.Hour

type Opts struct {
	fx.In
	Log      logger.Logger
	Mapper   NotificationMapper
	Database *mongo.Database
}

type Repository struct {
	log    logger.Logger
	mapper NotificationMapper
	coll   *mongo.Collection
}

// notificationIndexes are the indexes the notifications collection needs.
//
// The list index matches ListNotifications: the filter is
// $or [{user_id: <caller>}, {user_id: "broadcast"}] and the sort is
// {created_at: -1, _id: -1} (orderby's created_at desc default plus the _id
// tiebreaker pagination.List appends). Each $or leg pins user_id by equality, so
// the leg reads the index in created_at desc, _id desc order and the planner
// merges the two legs instead of sorting in memory. Without this index every
// ListNotifications call scanned a collection that grows with every notification
// any domain writes, and the response is capped at one page regardless.
//
// The TTL is deliberately whole-collection, not partial on {state: "read"}. A
// read-only partial TTL never expires the documents an attacker writes, because
// those are unread by definition and only the victim can mark them read: the
// collection would still grow without bound and still be scanned, which is the
// finding this index closes. The price is that a notification nobody read within
// retention expires too; 90 days after it was written it is stale in an inbox
// that pages 15 at a time.
//
// Created one at a time, like the lfg repository: CreateMany is all-or-nothing,
// so a single conflicting spec would also lose the TTL index.
var notificationIndexes = []mongo.IndexModel{
	// The inbox read: user_id equality per $or leg, then the sort keys.
	{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}},
	// Retention. created_at comes from mongox.Model.
	{
		Keys:    bson.D{{Key: "created_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(int32(retention / time.Second)),
	},
}

func New(opts Opts) *Repository {
	log := opts.Log.WithComponent("repository")
	coll := opts.Database.Collection(collName)
	setupIndexes(log, coll)

	return &Repository{
		log:    log,
		mapper: opts.Mapper,
		coll:   coll,
	}
}

func setupIndexes(log logger.Logger, coll *mongo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, model := range notificationIndexes {
		if _, err := coll.Indexes().CreateOne(ctx, model); err != nil {
			log.Error("failed to create index",
				zap.String("collection", collName),
				zap.Error(err),
			)
		}
	}
}
