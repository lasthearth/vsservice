package repository

import (
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/rules/internal/service"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
)

const (
	questionsCollName = "questions"
)

var _ service.DbRepository = (*Repository)(nil)

type Opts struct {
	fx.In
	Log      logger.Logger
	Cfg      config.Config
	Database *mongo.Database
}

type Repository struct {
	log          logger.Logger
	cfg          config.Config
	questionColl *mongo.Collection
}

func New(opts Opts) *Repository {
	qColl := opts.Database.Collection(questionsCollName)
	logger := opts.Log.WithComponent("rules-mongo-repository")
	return &Repository{
		log:          logger,
		cfg:          opts.Cfg,
		questionColl: qColl,
	}
}
