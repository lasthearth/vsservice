package repository

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/player/internal/event"
	service "github.com/lasthearth/vsservice/internal/player/internal/service/player"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const collName = "verification_requests"

var (
	_ service.DbRepository   = (*Repository)(nil)
	_ event.PlayerRepository = (*Repository)(nil)
)

type Opts struct {
	fx.In
	Database *mongo.Database
	Logger   logger.Logger
	Mapper   Mapper
	Config   config.Config
}

type Repository struct {
	log    logger.Logger
	coll   *mongo.Collection
	mapper Mapper
	cfg    config.Config
}

func New(opts Opts) *Repository {
	coll := opts.Database.Collection(collName)
	logger := opts.Logger.WithComponent("user-mongo-repository")
	setupIndexes(logger, coll)
	return &Repository{
		log:    logger,
		coll:   coll,
		mapper: opts.Mapper,
		cfg:    opts.Config,
	}
}

func setupIndexes(log logger.Logger, coll *mongo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The dominant lookup: by the SSO user id (this repository's FindOne and
	// the cross-domain readers, e.g. appearance standings). Not unique on
	// purpose: the check-then-create Submit flow can already have left two
	// documents for one user, and a unique index would then fail to build.
	if _, err := coll.Indexes().CreateOne(
		ctx,
		mongo.IndexModel{Keys: bson.D{{Key: "user_id", Value: 1}}},
	); err != nil {
		log.Error("failed to create index", zap.String("collection", collName), zap.Error(err))
	}
}
