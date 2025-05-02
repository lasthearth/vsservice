package server

import (
	"context"
	"net"
	"net/http"

	"github.com/go-faster/errors"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/selector"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	leaderboardv1 "github.com/lasthearth/vsservice/gen/leaderboard/v1"
	v1 "github.com/lasthearth/vsservice/gen/proto/v1"
	rulesv1 "github.com/lasthearth/vsservice/gen/rules/v1"
	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/rs/cors"
	"github.com/tmc/grpc-websocket-proxy/wsproxy"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

func (s *Server) Run(ctx context.Context, network, address string) error {
	l, err := net.Listen(network, address)
	if err != nil {
		return err
	}
	defer func() {
		if err := l.Close(); err != nil {
			grpclog.Errorf("Failed to close %s %s: %v", network, address, err)
		}
	}()
	logOpts := []logging.Option{
		logging.WithLogOnEvents(
			logging.StartCall, logging.FinishCall,
			logging.PayloadSent, logging.PayloadReceived,
		),
	}

	recoveryOpts := []recovery.Option{
		recovery.WithRecoveryHandler(func(p any) (err error) {
			s.log.Error("Recovered from panic", zap.Any("panic", p))
			return status.Error(codes.Internal, "Internal server error")
		}),
	}

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			selector.UnaryServerInterceptor(s.authInterceptor.Unary(), selector.MatchFunc(interceptor.AuthMatcher)),
			recovery.UnaryServerInterceptor(recoveryOpts...),
			logging.UnaryServerInterceptor(interceptorLogger(s.log), logOpts...),
		),
		grpc.ChainStreamInterceptor(
			selector.StreamServerInterceptor(s.authInterceptor.Stream(), selector.MatchFunc(interceptor.AuthMatcher)),
			recovery.StreamServerInterceptor(recoveryOpts...),
			logging.StreamServerInterceptor(interceptorLogger(s.log), logOpts...),
		),
	)

	v1.RegisterVintageServiceServer(srv, s.vsApiV1)
	leaderboardv1.RegisterLeaderboardServiceServer(srv, s.leaderboardV1)
	rulesv1.RegisterRuleServiceServer(srv, s.rulesV1)
	userv1.RegisterUserServiceServer(srv, s.userV1)
	reflection.Register(srv)

	s.grpcSrv = srv
	return srv.Serve(l)
}

// RunInProcessGateway starts the invoke in process http gateway.
func (s *Server) RunInProcessGateway(ctx context.Context, addr string, opts ...runtime.ServeMuxOption) error {
	mux := runtime.NewServeMux(opts...)

	if err := v1.RegisterVintageServiceHandlerServer(ctx, mux, s.vsApiV1); err != nil {
		return errors.Wrap(err, "register vintage service handler")
	}

	dopts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	if err := leaderboardv1.RegisterLeaderboardServiceHandlerFromEndpoint(ctx, mux, addr, dopts); err != nil {
		return errors.Wrap(err, "register leaderboard service handler")
	}

	if err := rulesv1.RegisterRuleServiceHandlerFromEndpoint(ctx, mux, addr, dopts); err != nil {
		return errors.Wrap(err, "register rules service handler")
	}

	if err := userv1.RegisterUserServiceHandlerFromEndpoint(ctx, mux, addr, dopts); err != nil {
		return errors.Wrap(err, "register user service handler")
	}

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
		Debug:            s.c.AppEnv != "prod",
	}).Handler(mux)

	wshandler := wsproxy.WebsocketProxy(handler)

	srv := &http.Server{
		Addr:    addr,
		Handler: wshandler,
	}

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		grpclog.Errorf("Failed to listen and serve: %v", err)
		return err
	}

	s.gwSrv = srv
	return nil
}

func (s *Server) GracefulStop(ctx context.Context) {
	grpclog.Infof("Shutting down the server")

	if s.grpcSrv != nil {
		s.grpcSrv.GracefulStop()
	}

	if s.gwSrv != nil {
		if err := s.gwSrv.Shutdown(ctx); err != nil {
			grpclog.Errorf("Failed to shutdown http gateway server: %v", err)
		}
	}
}

// interceptorLogger Retrieved from
// https://github.com/grpc-ecosystem/go-grpc-middleware/blob/62b7de50cda5a5d633f1013bfbe50e0f38db34ef/interceptors/logging/examples/zap/example_test.go#L17
func interceptorLogger(l logger.Logger) logging.Logger {
	return l.ToLoggingLogger()
}
