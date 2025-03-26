package server

import (
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	v1 "github.com/ripls56/vsservice/gen/proto/v1"
	"github.com/ripls56/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

type Opts struct {
	fx.In
	Log           logger.Logger
	VsApiV1       v1.VintageServiceServer
	LeaderboardV1 v1.LeaderboardServiceServer
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
	v1.RegisterLeaderboardServiceServer(srv, opts.LeaderboardV1)
	reflection.Register(srv)

	return &GrpcServer{Srv: srv}
}
