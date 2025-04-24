package sso

import (
	"context"
	"net/http"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/tokenmanager"
	"github.com/lasthearth/vsservice/internal/rules/internal/service"
	"go.uber.org/fx"
)

var _ service.SsoRepository = (*Repository)(nil)

type Opts struct {
	fx.In
	Manager *tokenmanager.Manager
	Cfg     config.Config
}

type Repository struct {
	client *http.Client
	cfg    config.Config
}

func New(opts Opts) *Repository {
	return &Repository{
		client: opts.Manager.Client(context.Background()),
		cfg:    opts.Cfg,
	}
}
