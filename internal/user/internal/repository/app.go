package repository

import (
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/user/internal/service"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
)

const collName = "verification_requests"

var _ service.Repository = (*Repository)(nil)

type Opts struct {
	fx.In
	Database *mongo.Database
	Logger   logger.Logger
}

type Repository struct {
	log  logger.Logger
	coll *mongo.Collection
}

func New(opts Opts) *Repository {
	coll := opts.Database.Collection(collName)
	return &Repository{
		log:  opts.Logger,
		coll: coll,
	}
}
