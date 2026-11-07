package repository

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	service "github.com/lasthearth/vsservice/internal/player/internal/service/player"
	"github.com/lasthearth/vsservice/internal/player/internal/service/presence"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	collName = "verification_requests"
	// statsCollName is the game's stats collection. The roster joins it by
	// user_game_name the way the leaderboard does, so both show the same hours
	// and the same last_online.
	statsCollName = "stats"

	// indexSetupTimeout bounds both index builds together.
	indexSetupTimeout = 10 * time.Second
)

var (
	_ service.DbRepository = (*Repository)(nil)
	_ presence.Repository  = (*Repository)(nil)
)

// playerIndexes are the indexes the verification_requests collection needs.
var playerIndexes = []mongo.IndexModel{
	// The dominant lookup: by the SSO user id (this repository's FindOne and
	// the cross-domain readers, e.g. appearance standings). Not unique on
	// purpose: the check-then-create Submit flow can already have left two
	// documents for one user, and a unique index would then fail to build.
	{Keys: bson.D{{Key: "user_id", Value: 1}}},
	// The roster joins this collection on the game name, and so does the
	// leaderboard. Without an index the $lookup scans stats once per player.
	{Keys: bson.D{{Key: "user_game_name", Value: 1}}},
	// The presence sweeper's filter: an equality on is_online and a range on
	// last_heartbeat. Partial on is_online: true because the collection is large
	// and the documents claiming to be online are dozens. A partial index is
	// only usable by a query that carries its filter expression, which
	// buildStalePresenceFilter does; index_presence_test.go pins that agreement,
	// because the mismatch is silent — MongoDB answers from a collection scan
	// and nothing says so.
	{
		Keys:    bson.D{{Key: "is_online", Value: 1}, {Key: "last_heartbeat", Value: 1}},
		Options: options.Index().SetPartialFilterExpression(bson.M{"is_online": true}),
	},
}

// statsIndexes are the indexes the game's stats collection needs. The game
// writes this collection and nothing else indexes it, so they are created here,
// next to the collection handle that reads it.
var statsIndexes = []mongo.IndexModel{
	// The join key the roster and the leaderboard look stats up by.
	{Keys: bson.D{{Key: "user_game_name", Value: 1}}},
}

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

	// One budget for both collections. The constructor runs inside fx startup,
	// whose default timeout is 15 seconds: two sequential 10-second waits would
	// abort the process instead of starting without an index.
	ctx, cancel := context.WithTimeout(context.Background(), indexSetupTimeout)
	defer cancel()

	setupIndexes(ctx, logger, coll)
	setupStatsIndexes(ctx, logger, opts.Database.Collection(statsCollName))
	return &Repository{
		log:    logger,
		coll:   coll,
		mapper: opts.Mapper,
		cfg:    opts.Config,
	}
}

func setupIndexes(ctx context.Context, log logger.Logger, coll *mongo.Collection) {
	if _, err := coll.Indexes().CreateMany(ctx, playerIndexes); err != nil {
		log.Error("failed to create indexes", zap.String("collection", collName), zap.Error(err))
	}
}

// setupStatsIndexes creates the indexes the game's stats collection needs. The
// game writes this collection, and nothing else indexes it, so they are created
// here, next to the collection handle that reads it.
func setupStatsIndexes(ctx context.Context, log logger.Logger, coll *mongo.Collection) {
	if _, err := coll.Indexes().CreateMany(ctx, statsIndexes); err != nil {
		log.Error("failed to create indexes", zap.String("collection", statsCollName), zap.Error(err))
	}
}
