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

const collName = "events"

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

	if _, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "until", Value: 1}},
	}); err != nil {
		log.Error("failed to create index", zap.String("collection", collName), zap.Error(err))
	}
}
