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
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	collName = "verification_requests"
	// statsCollName is the game's stats collection. The roster joins it by
	// user_game_name the way the leaderboard does, so both show the same hours
	// and the same last_online.
	statsCollName = "stats"
)

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
	setupStatsIndexes(logger, opts.Database.Collection(statsCollName))
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

	// The roster joins this collection on the game name, and so does the
	// leaderboard. Without an index the $lookup scans stats once per player.
	if _, err := coll.Indexes().CreateOne(
		ctx,
		mongo.IndexModel{Keys: bson.D{{Key: "user_game_name", Value: 1}}},
	); err != nil {
		log.Error("failed to create index", zap.String("collection", collName), zap.Error(err))
	}
}

// setupStatsIndexes creates the join key the roster and the leaderboard look
// stats up by. The game writes this collection, and nothing else indexes it, so
// the index is created here, next to the collection handle that reads it.
func setupStatsIndexes(log logger.Logger, coll *mongo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := coll.Indexes().CreateOne(
		ctx,
		mongo.IndexModel{Keys: bson.D{{Key: "user_game_name", Value: 1}}},
	); err != nil {
		log.Error("failed to create index", zap.String("collection", statsCollName), zap.Error(err))
	}
}
