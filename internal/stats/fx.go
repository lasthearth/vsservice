package stats

import (
	"context"
	"github.com/eapache/go-resiliency/retrier"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	vsservice "github.com/lasthearth/vsservice/internal/service"
	"github.com/lasthearth/vsservice/internal/stats/internal/repository"
	"github.com/lasthearth/vsservice/internal/stats/internal/service"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const module = "stats"

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
				repository.New,
				fx.As(new(service.Repository)),
			),
		),

		fx.Provide(
			service.New,
			fx.Annotate(service.New, fx.As(new(vsservice.StatsService))),
		),

		fx.Invoke(
			func(c config.Config, lc fx.Lifecycle, log logger.Logger, service *service.Service, retrier *retrier.Retrier) {
				lc.Append(
					fx.Hook{
						OnStart: func(ctx context.Context) error {
							if !c.StatsFetchingEnable {
								return nil
							}
							go func() {
								err := retrier.Run(func() error {
									err := service.StartFetching(context.Background())
									if err != nil {
										log.Error("fetching failed", zap.Error(err))
										return err
									}

									log.Info("fetching started")
									return nil
								})

								if err != nil {
									panic(err)
								}
							}()
							return nil
						},
						OnStop: func(ctx context.Context) error {
							if !c.StatsFetchingEnable {
								return nil
							}
							service.StopFetching()
							return nil
						},
					},
				)
			},
		),
	),
)
