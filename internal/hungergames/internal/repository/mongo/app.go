package repository

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/hungergames/internal/service"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	mgo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	playerStatsCollName   = "hg_player_stats"
	seasonsCollName       = "hg_seasons"
	seasonResultsCollName = "hg_season_results"
	// Cross-domain: donate wallet and transaction collections
	donateWalletCollName = "donate_wallets"
	donateTxCollName     = "donate_transactions"
)

var _ service.Repository = (*Repository)(nil)

// Repository implements service.Repository by writing to MongoDB.
type Repository struct {
	log              logger.Logger
	playerStatsColl  *mgo.Collection
	seasonsColl      *mgo.Collection
	seasonResultColl *mgo.Collection
	walletColl       *mgo.Collection
	txColl           *mgo.Collection
}

type Opts struct {
	fx.In

	Log      logger.Logger
	Database *mgo.Database
}

func New(opts Opts) *Repository {
	log := opts.Log.WithComponent("hungergames-repository")

	playerStatsColl := opts.Database.Collection(playerStatsCollName)
	seasonsColl := opts.Database.Collection(seasonsCollName)
	seasonResultColl := opts.Database.Collection(seasonResultsCollName)
	walletColl := opts.Database.Collection(donateWalletCollName)
	txColl := opts.Database.Collection(donateTxCollName)

	setupIndexes(log, playerStatsColl, seasonsColl, seasonResultColl)

	return &Repository{
		log:              log,
		playerStatsColl:  playerStatsColl,
		seasonsColl:      seasonsColl,
		seasonResultColl: seasonResultColl,
		walletColl:       walletColl,
		txColl:           txColl,
	}
}

func setupIndexes(log logger.Logger, playerStatsColl, seasonsColl, seasonResultColl *mgo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	createIndex := func(coll *mgo.Collection, model mgo.IndexModel) {
		if _, err := coll.Indexes().CreateOne(ctx, model); err != nil {
			log.Error("failed to create index", zap.String("collection", coll.Name()), zap.Error(err))
		}
	}

	// Unique per player per season
	createIndex(playerStatsColl, mgo.IndexModel{
		Keys:    bson.D{{Key: "player_id", Value: 1}, {Key: "season_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	// Fast ELO-sorted leaderboard queries
	createIndex(playerStatsColl, mgo.IndexModel{
		Keys: bson.D{{Key: "elo", Value: -1}},
	})

	// Only one active season at a time
	createIndex(seasonsColl, mgo.IndexModel{
		Keys: bson.D{{Key: "ended_at", Value: 1}},
	})
	createIndex(seasonsColl, mgo.IndexModel{
		Keys:    bson.D{{Key: "number", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	// Fast lookups by season
	createIndex(seasonResultColl, mgo.IndexModel{
		Keys: bson.D{{Key: "season_id", Value: 1}, {Key: "rank", Value: 1}},
	})
	createIndex(seasonResultColl, mgo.IndexModel{
		Keys:    bson.D{{Key: "season_id", Value: 1}, {Key: "player_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
}

// newModel is a local alias to avoid repeating the package path everywhere.
func newModel() mongox.Model { return mongox.NewModel() }
