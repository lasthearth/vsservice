package server

import (
	"context"
	"fmt"
	"strconv"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	v1 "github.com/lasthearth/vsservice/gen/proto/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/jwt"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/service"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
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
			func(c config.Config) (keyfunc.Keyfunc, error) {
				return keyfunc.NewDefault([]string{c.JWKS_URL})
			},
			jwt.NewManager,
		),

		fx.Provide(
			interceptor.NewAuth,

			fx.Annotate(service.NewVsApiV1, fx.As(new(v1.VintageServiceServer))),
			New,
		),

		fx.Invoke(
			func(lc fx.Lifecycle, log logger.Logger, c config.Config, server *Server) {
				lc.Append(
					fx.Hook{
						OnStart: func(ctx context.Context) error {
							go func() {
								addr := fmt.Sprintf(":%d", c.GrpcPort)
								err := server.Run(context.Background(), "tcp", addr)
								if err != nil {
									log.Error("server stopped", zap.Error(err))
								}
							}()

							go func() {
								grpcaddr := fmt.Sprintf(":%d", c.GrpcPort)
								addr := fmt.Sprintf(":%d", c.GateAwayPort)
								err := server.RunInProcessGateway(
									context.Background(),
									grpcaddr,
									addr,
									runtime.WithMarshalerOption(
										runtime.MIMEWildcard, &runtime.JSONPb{
											MarshalOptions: protojson.MarshalOptions{
												UseProtoNames:     true,
												EmitDefaultValues: true,
											},
											UnmarshalOptions: protojson.UnmarshalOptions{},
										},
									),
								)
								if err != nil {
									log.Error("gw server stopped", zap.Error(err))
								}
							}()

							log.Info("server started", zap.String("port", strconv.Itoa(c.GrpcPort)))
							return nil
						},
						OnStop: func(ctx context.Context) error {
							log.Info("gracefully stopping grpc server...")
							server.GracefulStop(context.Background())

							log.Info("server stopped")
							return nil
						},
					},
				)
			},
		),
	),
)
