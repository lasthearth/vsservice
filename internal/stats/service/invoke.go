package service

import (
	"context"

	"github.com/eapache/go-resiliency/retrier"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

var App = fx.Options(
	fx.Provide(New, fx.Private),
	fx.Invoke(
		func(lc fx.Lifecycle, log logger.Logger, service *Service, retrier *retrier.Retrier) {
			lc.Append(
				fx.Hook{
					OnStart: func(ctx context.Context) error {
						go func() {
							err := retrier.Run(func() error {
								err := service.startFetching(context.Background())
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
						service.stopFetching()
						return nil
					},
				},
			)
		},
	),
)
