package service

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
	Repo   Repository
}

type Service struct {
	client http.Client
	log    logger.Logger
	cfg    config.Config
	repo   Repository
}

func New(opts Opts) *Service {
	return &Service{
		client: opts.Client,
		log:    opts.Log,
		cfg:    opts.Cfg,
		repo:   opts.Repo,
	}
}
