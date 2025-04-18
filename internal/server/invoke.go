package server

import (
	"context"
	v1 "github.com/lasthearth/vsservice/gen/proto/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/service"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"strconv"
)

const module = "grpc"

var App = fx.Options(
	fx.Module(
		module,
		fx.Decorate(
			func(l logger.Logger) logger.Logger {
				return l.WithScope(module)
			},
		),

		fx.Provide(
			fx.Annotate(service.NewVsApiV1, fx.As(new(v1.VintageServiceServer))),
			New,
		),

		fx.Invoke(
			func(lc fx.Lifecycle, log logger.Logger, c config.Config, server *GrpcServer) {
				lc.Append(
					fx.Hook{
						OnStart: func(ctx context.Context) error {
							go func() {
								err := server.Run(context.Background(), c)
								if err != nil {
									log.Error("server stopped", zap.Error(err))
								}

								log.Info("server started on port", zap.String("addr", strconv.Itoa(c.GrpcPort)))

							}()
							return nil
						},
						OnStop: func(ctx context.Context) error {
							log.Info("gracefully stopping grpc server...")
							server.GracefulStop()

							log.Info("server stopped")
							return nil
						},
					},
				)
			},
		),
	),
)
