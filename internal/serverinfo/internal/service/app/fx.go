package servicefx

import (
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/serverinfo/internal/service/serverinfo"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In
	Log  logger.Logger
	Repo serverinfo.ServerInfoRepository
}

func NewServiceFx(opts Opts) *serverinfo.Service {
	return serverinfo.NewService(opts.Log, opts.Repo)
}
