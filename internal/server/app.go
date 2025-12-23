package server

import (
	"net/http"

	"github.com/lasthearth/vsservice/internal/webhook"

	kitv1 "github.com/lasthearth/vsservice/gen/kit/v1"
	leaderboardv1 "github.com/lasthearth/vsservice/gen/leaderboard/v1"
	newsv1 "github.com/lasthearth/vsservice/gen/news/v1"
	notificationv1 "github.com/lasthearth/vsservice/gen/notification/v1"
	serverinfov1 "github.com/lasthearth/vsservice/gen/serverinfo/v1"

	rulesv1 "github.com/lasthearth/vsservice/gen/rules/v1"
	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	verificationv1 "github.com/lasthearth/vsservice/gen/verification/v1"
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

	Log             logger.Logger
	LeaderboardV1   leaderboardv1.LeaderboardServiceServer
	RulesV1         rulesv1.RuleServiceServer
	VerificationV1  verificationv1.VerificationServiceServer
	UserV1          userv1.UserServiceServer
	SettlementV1    settlementv1.SettlementServiceServer
	SettlementTagV1 settlementv1.SettlementTagServiceServer
	NotificationV1  notificationv1.NotificationServiceServer
	NewsV1          newsv1.NewsServiceServer
	KitV1           kitv1.KitServiceServer
	ServerInfoV1    serverinfov1.ServerInfoServiceServer
	// Add the webhook service
	LogtoWebhookService *webhook.LogtoWebhookService
}

type Server struct {
	authInterceptor *interceptor.Auth

	c                   config.Config
	leaderboardV1       leaderboardv1.LeaderboardServiceServer
	rulesV1             rulesv1.RuleServiceServer
	verificationV1      verificationv1.VerificationServiceServer
	userV1              userv1.UserServiceServer
	settlementV1        settlementv1.SettlementServiceServer
	settlementTagV1     settlementv1.SettlementTagServiceServer
	notificationV1      notificationv1.NotificationServiceServer
	newsV1              newsv1.NewsServiceServer
	kitV1               kitv1.KitServiceServer
	serverInfoV1        serverinfov1.ServerInfoServiceServer
	logtoWebhookService *webhook.LogtoWebhookService

	log logger.Logger

	// runtime
	grpcSrv *grpc.Server
	gwSrv   *http.Server
}

func New(opts Opts) *Server {
	return &Server{
		authInterceptor:     opts.AuthInterceptor,
		c:                   opts.Config,
		leaderboardV1:       opts.LeaderboardV1,
		rulesV1:             opts.RulesV1,
		verificationV1:      opts.VerificationV1,
		userV1:              opts.UserV1,
		settlementV1:        opts.SettlementV1,
		settlementTagV1:     opts.SettlementTagV1,
		notificationV1:      opts.NotificationV1,
		newsV1:              opts.NewsV1,
		kitV1:               opts.KitV1,
		serverInfoV1:        opts.ServerInfoV1,
		logtoWebhookService: opts.LogtoWebhookService,
		log:                 opts.Log,
	}
}
