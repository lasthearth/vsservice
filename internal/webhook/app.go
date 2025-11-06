package webhook

import (
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In

	Config config.Config
	Log    logger.Logger
}

var App = fx.Options(
	fx.Provide(NewLogtoWebhookServiceWithOpts),
)

func NewLogtoWebhookServiceWithOpts(opts Opts) *LogtoWebhookService {
	return NewLogtoWebhookService(opts.Log, opts.Config)
}
