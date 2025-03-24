package repository

import (
	"github.com/ripls56/vsservice/config"
	"github.com/ripls56/vsservice/logger"
	"go.uber.org/fx"
	"net/http"
)

type Opts struct {
	fx.In
	Client http.Client
	Log    logger.Logger
	Cfg    config.Config
}

type Repository struct {
	client http.Client
	log    logger.Logger
	cfg    config.Config
}

func New(opts Opts) *Repository {
	return &Repository{
		client: opts.Client,
		log:    opts.Log,
		cfg:    opts.Cfg,
	}
}
