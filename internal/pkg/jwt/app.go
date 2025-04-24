package jwt

import (
	"github.com/MicahParks/keyfunc/v3"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In
	Kfn keyfunc.Keyfunc
	Cfg config.Config
}

type Manager struct {
	kfn keyfunc.Keyfunc
	cfg config.Config
}

func NewManager(opts Opts) *Manager {
	return &Manager{
		kfn: opts.Kfn,
		cfg: opts.Cfg,
	}
}
