package sso

import (
	"context"
	"net/http"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/tokenmanager"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In
	Manager *tokenmanager.Manager
	Config  config.Config
}

type Repository struct {
	client *http.Client
	config config.Config
}

func New(opts Opts) *Repository {
	return &Repository{
		client: opts.Manager.Client(context.Background()),
		config: opts.Config,
	}
}
