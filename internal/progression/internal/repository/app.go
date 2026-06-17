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
	log          logger.Logger
	treesColl    *mongo.Collection
	presetsColl  *mongo.Collection
	progressColl *mongo.Collection
}

func New(opts Opts) *Repository {
	return &Repository{
		log:          opts.Log,
		treesColl:    opts.Database.Collection("talent_trees"),
		presetsColl:  opts.Database.Collection("talent_presets"),
		progressColl: opts.Database.Collection("talent_progress"),
	}
}
