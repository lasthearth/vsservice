package repository

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/kitdef/internal/service"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/bson"
	mgo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// kitsCollName is the shared collection owned jointly with the game. vsservice
// writes only metadata (title); the game writes code/items/created_at.
const kitsCollName = "kits"

var _ service.KitRepository = (*Repository)(nil)

type Repository struct {
	log      logger.Logger
	kitsColl *mgo.Collection
}

type Opts struct {
	fx.In

	Log      logger.Logger
	Database *mgo.Database
}

func New(opts Opts) *Repository {
	log := opts.Log.WithComponent("kitdef-repository")
	kitsColl := opts.Database.Collection(kitsCollName)
	setupIndexes(log, kitsColl)
	return &Repository{
		log:      log,
		kitsColl: kitsColl,
	}
}

func setupIndexes(log logger.Logger, kitsColl *mgo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// code is the game's upsert key — unique across the collection.
	if _, err := kitsColl.Indexes().CreateOne(ctx, mgo.IndexModel{
		Keys:    bson.D{{Key: "code", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		log.Error("failed to create index", zap.String("collection", kitsColl.Name()), zap.Error(err))
	}
}
