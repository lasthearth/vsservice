package repository

import (
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/verification/internal/service"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
)

const (
	verificationCollName = "verification_requests"
)

var _ service.VerificationDbRepository = (*Repository)(nil)

type Opts struct {
	fx.In
	Log      logger.Logger
	Cfg      config.Config
	Database *mongo.Database
}

type Repository struct {
	log  logger.Logger
	cfg  config.Config
	coll *mongo.Collection
}

func New(opts Opts) *Repository {
	vColl := opts.Database.Collection(verificationCollName)
	logger := opts.Log.WithComponent("rules-mongo-repository")
	return &Repository{
		log:  logger,
		cfg:  opts.Cfg,
		coll: vColl,
	}
}
