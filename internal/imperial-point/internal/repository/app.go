package repository

import (
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
)

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
	return &Repository{
		log:  opts.Log,
		coll: opts.Database.Collection("imperial_points"),
	}
}
