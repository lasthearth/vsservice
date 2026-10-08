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

const (
	collName          = "appearances"
	purchasesCollName = "appearance_purchases"

	// Read-only: owned by other domains, read here to check unlocks.
	statsCollName           = "stats"
	playerCollName          = "verification_requests"
	settlementsCollName     = "settlements"
	hgStatsCollName         = "hg_player_stats"
	hgSeasonResultsCollName = "hg_season_results"
	referralsCollName       = "referral_events"
	attendeesCollName       = "event_attendees"
)

type Opts struct {
	fx.In
	Logger logger.Logger
	Db     *mongo.Database
	Mapper Mapper
}

// Repository stores appearances and the items bought for shards.
type Repository struct {
	logger    logger.Logger
	coll      *mongo.Collection
	purchases *mongo.Collection
	mapper    Mapper
}

func New(opts Opts) *Repository {
	l := opts.Logger.WithComponent("repository")
	coll := opts.Db.Collection(collName)
	purchases := opts.Db.Collection(purchasesCollName)
	setupIndexes(l, coll, purchases)

	return &Repository{
		logger:    l,
		coll:      coll,
		purchases: purchases,
		mapper:    opts.Mapper,
	}
}

func setupIndexes(log logger.Logger, coll, purchases *mongo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// One look per player.
	looks := []mongo.IndexModel{
		{Keys: bson.D{{Key: "user_id", Value: 1}}, Options: options.Index().SetUnique(true)},
	}
	if _, err := coll.Indexes().CreateMany(ctx, looks); err != nil {
		log.Error("failed to create indexes", zap.String("collection", collName), zap.Error(err))
	}

	// An item is bought once: the unique pair stops a double charge from two
	// clicks racing each other.
	bought := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "item_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	}
	if _, err := purchases.Indexes().CreateMany(ctx, bought); err != nil {
		log.Error("failed to create indexes", zap.String("collection", purchasesCollName), zap.Error(err))
	}
}

type StandingOpts struct {
	fx.In
	Logger logger.Logger
	Db     *mongo.Database
}

// Standings reads what unlocks appearance items: game stats, players,
// settlements, Hunger Games, referrals, event sign-ups and shard purchases.
// Everything but the purchases is owned by other domains and only read here.
type Standings struct {
	logger        logger.Logger
	stats         *mongo.Collection
	players       *mongo.Collection
	settlements   *mongo.Collection
	hgStats       *mongo.Collection
	seasonResults *mongo.Collection
	referrals     *mongo.Collection
	attendees     *mongo.Collection
	purchases     *mongo.Collection
	now           func() time.Time
}

func NewStandings(opts StandingOpts) *Standings {
	return &Standings{
		logger:        opts.Logger.WithComponent("standings"),
		stats:         opts.Db.Collection(statsCollName),
		players:       opts.Db.Collection(playerCollName),
		settlements:   opts.Db.Collection(settlementsCollName),
		hgStats:       opts.Db.Collection(hgStatsCollName),
		seasonResults: opts.Db.Collection(hgSeasonResultsCollName),
		referrals:     opts.Db.Collection(referralsCollName),
		attendees:     opts.Db.Collection(attendeesCollName),
		purchases:     opts.Db.Collection(purchasesCollName),
		now:           time.Now,
	}
}
