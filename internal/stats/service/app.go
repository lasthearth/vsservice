package service

import (
	"github.com/ripls56/vsservice/internal/pkg/config"
	"github.com/ripls56/vsservice/internal/pkg/logger"
	"github.com/ripls56/vsservice/internal/stats/internal/fetcher"
	"go.uber.org/fx"
	"net/http"
)

type Opts struct {
	fx.In
	Client *http.Client
	Log    logger.Logger
	Cfg    config.Config
	Repo   Repository
}

type Service struct {
	client  *http.Client
	log     logger.Logger
	cfg     config.Config
	repo    Repository
	fetcher *fetcher.Fetcher
}

func New(opts Opts) *Service {
	return &Service{
		client:  opts.Client,
		log:     opts.Log,
		cfg:     opts.Cfg,
		repo:    opts.Repo,
		fetcher: fetcher.New(opts.Log, opts.Cfg, opts.Client),
	}
}
