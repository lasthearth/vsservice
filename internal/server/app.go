package server

import (
	"net/http"

	leaderboardv1 "github.com/lasthearth/vsservice/gen/leaderboard/v1"
	v1 "github.com/lasthearth/vsservice/gen/proto/v1"
	rulesv1 "github.com/lasthearth/vsservice/gen/rules/v1"
	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
	"google.golang.org/grpc"
)

type Opts struct {
	fx.In

	AuthInterceptor *interceptor.Auth

	Config config.Config

	Log           logger.Logger
	VsApiV1       v1.VintageServiceServer
	LeaderboardV1 leaderboardv1.LeaderboardServiceServer
	RulesV1       rulesv1.RuleServiceServer
	UserV1        userv1.UserServiceServer
}

type Server struct {
	authInterceptor *interceptor.Auth

	c             config.Config
	vsApiV1       v1.VintageServiceServer
	leaderboardV1 leaderboardv1.LeaderboardServiceServer
	rulesV1       rulesv1.RuleServiceServer
	userV1        userv1.UserServiceServer
	log           logger.Logger

	// runtime
	grpcSrv *grpc.Server
	gwSrv   *http.Server
}

func New(opts Opts) *Server {
	return &Server{
		authInterceptor: opts.AuthInterceptor,
		c:               opts.Config,
		vsApiV1:         opts.VsApiV1,
		leaderboardV1:   opts.LeaderboardV1,
		rulesV1:         opts.RulesV1,
		userV1:          opts.UserV1,
		log:             opts.Log,
	}
}
