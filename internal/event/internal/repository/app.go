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
