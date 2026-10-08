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

const collName = "lfg_posts"

type Opts struct {
	fx.In
	Logger logger.Logger
	Db     *mongo.Database
	Mapper Mapper
}

type Repository struct {
	logger logger.Logger
	coll   *mongo.Collection
	mapper Mapper
}

func New(opts Opts) *Repository {
	l := opts.Logger.WithComponent("repository")
	coll := opts.Db.Collection(collName)
	setupIndexes(l, coll)

	return &Repository{
		logger: l,
		coll:   coll,
		mapper: opts.Mapper,
	}
}

// lfgIndexes are the indexes the lfg_posts collection needs.
//
// The two board indexes must match their sort exactly, key for key and
// direction for direction. ListOpen filters kind by equality, ranges on
// expires_at and checks closed_at, then sorts on {starts_at, _id} or
// {bumped_at, _id}. Putting the expires_at range in the middle of the index
// stops it from yielding the sort order, and MongoDB does not treat _id as an
// implicit index suffix, so either mistake makes the planner fall back to a
// blocking in-memory sort on a public endpoint. Leading with the kind equality
// and following with the sort keys keeps the sort indexed; expires_at,
// closed_at and activities are then residual filters, which is cheap here
// because the delete_at TTL below keeps only open and recently expired posts.
//
// Created one at a time: CreateMany is all-or-nothing, so a single conflicting
// spec would also lose the TTL index, and lfg_posts would then grow forever.
var lfgIndexes = []mongo.IndexModel{
	// The session board: open sessions in start order.
	{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "starts_at", Value: 1}, {Key: "_id", Value: 1}}},
	// The teammate board: open posts, freshest first.
	{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "bumped_at", Value: -1}, {Key: "_id", Value: -1}}},
	// The open-post limits per author and kind: equality on author and kind,
	// range on expires_at, no sort.
	{Keys: bson.D{{Key: "author_id", Value: 1}, {Key: "kind", Value: 1}, {Key: "expires_at", Value: 1}}},
	// Old posts delete themselves.
	{Keys: bson.D{{Key: "delete_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
}

func setupIndexes(log logger.Logger, coll *mongo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, model := range lfgIndexes {
		if _, err := coll.Indexes().CreateOne(ctx, model); err != nil {
			log.Error("failed to create index", zap.String("collection", collName), zap.Error(err))
		}
	}
}
