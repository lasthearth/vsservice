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

const collName = "reactions"

type Opts struct {
	fx.In
	Logger logger.Logger
	Db     *mongo.Database
}

type Repository struct {
	logger logger.Logger
	coll   *mongo.Collection
}

func New(opts Opts) *Repository {
	l := opts.Logger.WithComponent("repository")
	coll := opts.Db.Collection(collName)
	setupIndexes(l, coll)

	return &Repository{
		logger: l,
		coll:   coll,
	}
}

func setupIndexes(log logger.Logger, coll *mongo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	models := []mongo.IndexModel{
		// One reaction per (target, player, emoji): makes toggle race-safe.
		{
			Keys: bson.D{
				{Key: "target", Value: 1},
				{Key: "user_id", Value: 1},
				{Key: "emoji", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
		{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "target", Value: 1}}},
	}

	if _, err := coll.Indexes().CreateMany(ctx, models); err != nil {
		log.Error("failed to create indexes", zap.String("collection", collName), zap.Error(err))
	}
}
