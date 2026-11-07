package serverinfo

import (
	"context"

	serverinfov1 "github.com/lasthearth/vsservice/gen/serverinfo/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	repofx "github.com/lasthearth/vsservice/internal/serverinfo/internal/repository/app"
	servicefx "github.com/lasthearth/vsservice/internal/serverinfo/internal/service/app"
	"github.com/lasthearth/vsservice/internal/serverinfo/internal/service/serverinfo"
	"go.uber.org/fx"
)

var module = "serverinfo"

var App = fx.Options(
	fx.Module(
		module,
		fx.Decorate(
			func(l logger.Logger) logger.Logger {
				return l.WithScope(module)
			},
		),
		repofx.App,

		fx.Provide(
			fx.Private,
			servicefx.NewSweeperFx,
		),

		fx.Provide(
			fx.Annotate(servicefx.NewServiceFx,
				fx.As(new(serverinfov1.ServerInfoServiceServer)),
			),
		),

		// Zeroes a total_online whose heartbeat went stale: the game server owns
		// the document and stops writing when it dies.
		fx.Invoke(func(lc fx.Lifecycle, s *serverinfo.Sweeper) {
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			lc.Append(fx.Hook{
				OnStart: func(context.Context) error {
					go func() {
						defer close(done)
						s.Run(ctx)
					}()
					return nil
				},
				OnStop: func(stopCtx context.Context) error {
					cancel()
					select {
					case <-done:
					case <-stopCtx.Done():
					}
					return nil
				},
			})
		}),
	),
)
