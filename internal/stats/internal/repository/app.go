package repository

import (
	"github.com/ripls56/vsservice/internal/pkg/config"
	"github.com/ripls56/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
)

const collName = "stats"

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
	coll := opts.Database.Collection(collName)
	return &Repository{
		log:  opts.Log,
		cfg:  opts.Cfg,
		coll: coll,
	}
}
