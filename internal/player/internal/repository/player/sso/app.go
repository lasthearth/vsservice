package sso

import (
	"context"
	"net/http"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/tokenmanager"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In
	Manager *tokenmanager.Manager
	Logger  logger.Logger
	Cfg     config.Config
}

type Repository struct {
	client *http.Client
	cfg    config.Config
	logger logger.Logger
}

func New(opts Opts) *Repository {
	logger := opts.Logger.WithComponent("user-sso-repository")

	return &Repository{
		client: opts.Manager.Client(context.Background()),
		cfg:    opts.Cfg,
		logger: logger,
	}
}
