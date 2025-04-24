package server

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/go-faster/errors"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	v1 "github.com/lasthearth/vsservice/gen/proto/v1"
	rulesv1 "github.com/lasthearth/vsservice/gen/rules/v1"
	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/rs/cors"
	"github.com/tmc/grpc-websocket-proxy/wsproxy"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
)

func (a GrpcServer) Run(ctx context.Context, c config.Config) error {
	port := fmt.Sprintf(":%d", c.GrpcPort)
	httpPort := fmt.Sprintf(":%d", c.GateAwayPort)

	l, err := net.Listen("tcp", port)
	if err != nil {
		return errors.Wrap(err, "run")
	}

	var group errgroup.Group
	group.SetLimit(1)

	group.Go(func() error {
		return a.Srv.Serve(l)
	})

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	mux := runtime.NewServeMux(runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
		MarshalOptions: protojson.MarshalOptions{
			UseProtoNames:     true,
			EmitDefaultValues: true,
		},
		UnmarshalOptions: protojson.UnmarshalOptions{},
	}))

	group.Go(func() error {
		return v1.RegisterVintageServiceHandlerFromEndpoint(ctx, mux, port, opts)
	})

	group.Go(func() error {
		return v1.RegisterLeaderboardServiceHandlerFromEndpoint(ctx, mux, port, opts)
	})

	group.Go(func() error {
		return rulesv1.RegisterRuleServiceHandlerFromEndpoint(ctx, mux, port, opts)
	})

	group.Go(func() error {
		return userv1.RegisterUserServiceHandlerFromEndpoint(ctx, mux, port, opts)
	})

	group.Go(func() error {
		handler := cors.New(cors.Options{
			AllowedOrigins: []string{
				"https://lasthearth.ru",
				"http://localhost*",
				"http://0.0.0.0*",
				"https://*.lasthearth.ru",
			},
			AllowedHeaders: []string{
				"*",
			},
			AllowCredentials: true,

			Debug: true,
		}).Handler(mux)
		return http.ListenAndServe(httpPort, wsproxy.WebsocketProxy(handler))
	})

	return group.Wait()
}

func (a GrpcServer) GracefulStop() {
	a.Srv.GracefulStop()
}

// interceptorLogger Retrieved from
// https://github.com/grpc-ecosystem/go-grpc-middleware/blob/62b7de50cda5a5d633f1013bfbe50e0f38db34ef/interceptors/logging/examples/zap/example_test.go#L17
func interceptorLogger(l logger.Logger) logging.Logger {
	return l.ToLoggingLogger()
}
