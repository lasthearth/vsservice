package sso

import (
	"context"
	"net/http"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/tokenmanager"
	"github.com/lasthearth/vsservice/internal/verification/internal/service"
	"go.uber.org/fx"
)

var _ service.SsoRepository = (*Repository)(nil)

type Opts struct {
	fx.In
	Manager *tokenmanager.Manager
	Cfg     config.Config
	Logger  logger.Logger
}

type Repository struct {
	client *http.Client
	cfg    config.Config
	logger logger.Logger
}

func New(opts Opts) *Repository {
	logger := opts.Logger.WithComponent("rules-sso-repository")

	return &Repository{
		client: opts.Manager.Client(context.Background()),
		cfg:    opts.Cfg,
		logger: logger,
	}
}
