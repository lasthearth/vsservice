package repository

import (
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/settlement/internal/service"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
)

const (
	settlementCollName    = "settlements"
	settlementReqCollName = "settlement_requests"
)

var _ service.SettlementRepository = (*Repository)(nil)

type Opts struct {
	fx.In
	Log      logger.Logger
	Cfg      config.Config
	Database *mongo.Database
	Client   *mongo.Client
}

type Repository struct {
	log logger.Logger
	cfg config.Config
	// Settlements collection
	setColl *mongo.Collection
	// Settlement requests collection
	setReqColl *mongo.Collection
	// MongoDB client used for transactions
	client *mongo.Client
}

func New(opts Opts) *Repository {
	sColl := opts.Database.Collection(settlementCollName)
	srColl := opts.Database.Collection(settlementReqCollName)
	logger := opts.Log.WithComponent("settlement-mongo-repository")
	return &Repository{
		log:        logger,
		cfg:        opts.Cfg,
		setColl:    sColl,
		setReqColl: srColl,
		client:     opts.Client,
	}
}
