package playerfx

import (
	"context"

	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/storage"
	"github.com/lasthearth/vsservice/internal/player/internal/repository/player/repository"
	"github.com/lasthearth/vsservice/internal/player/internal/repository/player/repository/repomapper"
	"github.com/lasthearth/vsservice/internal/player/internal/repository/player/sso"
	service "github.com/lasthearth/vsservice/internal/player/internal/service/player"
	"github.com/lasthearth/vsservice/internal/player/internal/service/player/sermapper"
	"github.com/lasthearth/vsservice/internal/player/internal/service/presence"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

const module = "player"

var App = fx.Options(
	fx.Module(
		module,
		fx.Decorate(
			func(l logger.Logger) logger.Logger {
				return l.WithScope(module)
			},
		),

		fx.Provide(
			fx.Private,

			fx.Annotate(
				storage.New,
				fx.As(new(service.Storage)),
			),

			fx.Annotate(
				func() *repomapper.MapperImpl {
					return &repomapper.MapperImpl{}
				},
				fx.As(new(repository.Mapper)),
			),
			// One repository instance behind both interfaces: they read and
			// write the same collection, and a second construction would run
			// setupIndexes twice inside the same fx start budget.
			fx.Annotate(
				repository.New,
				fx.As(new(service.DbRepository)),
				fx.As(new(presence.Repository)),
			),

			fx.Annotate(
				sso.New,
				fx.As(new(service.SsoRepository)),
			),

			fx.Annotate(
				func() *sermapper.MapperImpl {
					return &sermapper.MapperImpl{}
				},
				fx.As(new(service.Mapper)),
			),

			presence.New,
		),

		fx.Provide(
			fx.Annotate(service.New,
				fx.As(new(userv1.UserServiceServer)),
			),
			fx.Annotate(service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
		),

		// Takes is_online off players whose game server stopped heartbeating.
		fx.Invoke(func(lc fx.Lifecycle, s *presence.Sweeper) {
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
