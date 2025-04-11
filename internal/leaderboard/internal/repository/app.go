package repository

import (
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
)

const collName = "stats"

type Opts struct {
	fx.In
	Log      logger.Logger
	Database *mongo.Database
}

type Repository struct {
	log  logger.Logger
	coll *mongo.Collection
}

func New(opts Opts) *Repository {
	coll := opts.Database.Collection(collName)
	return &Repository{
		log:  opts.Log,
		coll: coll,
	}
}
