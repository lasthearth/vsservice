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

const collName = "server_mods"

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

	// One document per mod: the id is what every lookup, the update and the
	// delete address, and a second row for one mod would let a reorder drop it.
	// Position is deliberately not indexed: the collection holds a few dozen
	// mods and the list is always read whole.
	if _, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "id", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		log.Error("failed to create index", zap.String("collection", collName), zap.Error(err))
	}
}
