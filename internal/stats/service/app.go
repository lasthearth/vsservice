package service

import (
	"net/http"

	"github.com/eapache/go-resiliency/retrier"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/stats/internal/fetcher"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In
	Client  *http.Client
	Log     logger.Logger
	Cfg     config.Config
	Retrier *retrier.Retrier
	Repo    Repository
}

type Service struct {
	client  *http.Client
	log     logger.Logger
	cfg     config.Config
	repo    Repository
	retrier *retrier.Retrier
	fetcher *fetcher.Fetcher
}

func New(opts Opts) *Service {
	return &Service{
		client:  opts.Client,
		log:     opts.Log,
		cfg:     opts.Cfg,
		repo:    opts.Repo,
		retrier: opts.Retrier,
		fetcher: fetcher.New(opts.Log, opts.Cfg, opts.Client),
	}
}
