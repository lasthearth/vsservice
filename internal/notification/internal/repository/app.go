package repository

import (
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In
	Log      logger.Logger
	Mapper   NotificationMapper
	Database *mongo.Database
}

type Repository struct {
	log    logger.Logger
	mapper NotificationMapper
	coll   *mongo.Collection
}

func New(opts Opts) *Repository {
	log := opts.Log.WithComponent("repository")
	return &Repository{
		log:    log,
		mapper: opts.Mapper,
		coll:   opts.Database.Collection("notifications"),
	}
}
