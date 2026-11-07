package servicefx

import (
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/serverinfo/internal/service/serverinfo"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In
	Log  logger.Logger
	Repo serverinfo.Repository
}

func NewServiceFx(opts Opts) *serverinfo.Service {
	return serverinfo.NewService(opts.Log, opts.Repo)
}

type SweeperOpts struct {
	fx.In
	Log  logger.Logger
	Repo serverinfo.Repository
	Cfg  config.Config
}

func NewSweeperFx(opts SweeperOpts) *serverinfo.Sweeper {
	return serverinfo.NewSweeper(opts.Log, opts.Repo, opts.Cfg)
}
