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

func setupIndexes(log logger.Logger, coll *mongo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	models := []mongo.IndexModel{
		// The session board: open sessions in start order.
		{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "expires_at", Value: 1}, {Key: "starts_at", Value: 1}}},
		// The teammate board: open posts, freshest first.
		{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "bumped_at", Value: -1}, {Key: "expires_at", Value: 1}}},
		// The open-post limits per author and kind.
		{Keys: bson.D{{Key: "author_id", Value: 1}, {Key: "kind", Value: 1}, {Key: "expires_at", Value: 1}}},
		// Old posts delete themselves.
		{Keys: bson.D{{Key: "delete_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
	}

	if _, err := coll.Indexes().CreateMany(ctx, models); err != nil {
		log.Error("failed to create indexes", zap.String("collection", collName), zap.Error(err))
	}
}
