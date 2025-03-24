package server

import (
	"context"
	"fmt"
	"github.com/go-faster/errors"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/ripls56/vsservice/config"
	v1 "github.com/ripls56/vsservice/gen/proto/v1"
	"github.com/ripls56/vsservice/logger"
	"github.com/rs/cors"
	"github.com/tmc/grpc-websocket-proxy/wsproxy"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"net"
	"net/http"
)

type Opts struct {
	fx.In
	Log     logger.Logger
	VsApiV1 v1.VintageServiceServer
}

type GrpcServer struct {
	Srv *grpc.Server
}

func New(opts Opts) *GrpcServer {
	logOpts := []logging.Option{
		logging.WithLogOnEvents(
			logging.StartCall, logging.FinishCall,
			logging.PayloadSent, logging.PayloadReceived,
		),
	}

	recoveryOpts := []recovery.Option{
		recovery.WithRecoveryHandler(func(p any) (err error) {
			opts.Log.Error("Recovered from panic", zap.Any("panic", p))
			return status.Error(codes.Internal, "Internal server error")
		}),
	}

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			recovery.UnaryServerInterceptor(recoveryOpts...),
			logging.UnaryServerInterceptor(interceptorLogger(opts.Log), logOpts...),
		),
		grpc.ChainStreamInterceptor(
			recovery.StreamServerInterceptor(recoveryOpts...),
			logging.StreamServerInterceptor(interceptorLogger(opts.Log), logOpts...),
		),
	)

	v1.RegisterVintageServiceServer(srv, opts.VsApiV1)
	reflection.Register(srv)

	return &GrpcServer{Srv: srv}
}

func (a GrpcServer) Run(ctx context.Context, c config.Config) error {
	port := fmt.Sprintf(":%d", c.GrpcPort)
	httpPort := fmt.Sprintf(":%d", c.GateAwayPort)

	l, err := net.Listen("tcp", port)
	if err != nil {
		return errors.Wrap(err, "run")
	}

	var group errgroup.Group

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
		handler := cors.New(cors.Options{
			AllowedOrigins:   []string{"https://lasthearth.ru", "http://localhost*", "http://0.0.0.0*"},
			AllowCredentials: true,
			Debug:            true,
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
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		f := make([]zap.Field, 0, len(fields)/2)

		for i := 0; i < len(fields); i += 2 {
			key := fields[i]
			value := fields[i+1]

			switch v := value.(type) {
			case string:
				f = append(f, zap.String(key.(string), v))
			case int:
				f = append(f, zap.Int(key.(string), v))
			case bool:
				f = append(f, zap.Bool(key.(string), v))
			default:
				f = append(f, zap.Any(key.(string), v))
			}
		}

		log := l.WithOptions(zap.AddCallerSkip(1)).With(f...)

		switch lvl {
		case logging.LevelDebug:
			log.Debug(msg)
		case logging.LevelInfo:
			log.Info(msg)
		case logging.LevelWarn:
			log.Warn(msg)
		case logging.LevelError:
			log.Error(msg)
		default:
			panic(fmt.Sprintf("unknown level %v", lvl))
		}
	})
}
