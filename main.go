package main

import (
	"net/http"
	"time"

	"github.com/eapache/go-resiliency/retrier"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/lasthearth/vsservice/internal/kit"
	"github.com/lasthearth/vsservice/internal/leaderboard"
	"github.com/lasthearth/vsservice/internal/news"
	"github.com/lasthearth/vsservice/internal/notification"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/pkg/tokenmanager"
	"github.com/lasthearth/vsservice/internal/player"
	"github.com/lasthearth/vsservice/internal/rules"
	"github.com/lasthearth/vsservice/internal/server"
	"github.com/lasthearth/vsservice/internal/settlement"
	settlementtag "github.com/lasthearth/vsservice/internal/settlement-tag"
	"github.com/lasthearth/vsservice/internal/stats"
	"github.com/lasthearth/vsservice/internal/webhook"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	// "github.com/lasthearth/vsservice/internal/trademarket"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	envDev  = "dev"
	envProd = "prod"
)

func main() {
	a := fx.New(
		fx.Provide(
			config.New,
			func(l logger.Logger) *http.Client {
				retryClient := retryablehttp.NewClient()
				retryClient.RetryMax = 5
				retryClient.HTTPClient.Timeout = time.Second * 3

				standardClient := retryClient.StandardClient()
				return standardClient
			},
			func() *retrier.Retrier {
				return retrier.New(
					retrier.ConstantBackoff(5, 300*time.Millisecond),
					nil,
				)
			},
			func(client *http.Client, c config.Config) *tokenmanager.Manager {
				return tokenmanager.NewManager(client, tokenmanager.Config{
					ClientID:     c.ClientID,
					ClientSecret: c.ClientSecret,
					TokenUrl:     c.TokenUrl,
					Resource:     c.Resource,
					Scopes:       c.Scopes,
				})
			},
			messaging.SetupConn,
			setupLogger,
			setupStorage,
			mongox.New,
			mongox.NewDatabase,
			// fx.Annotate(service.New, fx.As(new(vsservice.StatsService))),
		),

		leaderboard.App,
		stats.App,
		// trademarket.App,
		rules.App,
		player.App,
		server.App,
		settlement.App,
		settlementtag.App,
		notification.App,
		news.App,
		webhook.App,
		kit.App,
	)

	a.Run()
}

func setupLogger(c config.Config) (logger.Logger, error) {
	var zc zap.Config

	switch c.AppEnv {
	case envDev:
		zc = zap.NewDevelopmentConfig()
	case envProd:
		zc = zap.NewProductionConfig()
	default:
		zc = zap.NewDevelopmentConfig()
	}

	zc.OutputPaths = []string{"stdout"}
	zc.ErrorOutputPaths = []string{"stderr"}

	l, err := logger.New(&zc)
	if err != nil {
		return nil, err
	}
	return l, err
}

func setupStorage(c config.Config) (*minio.Client, error) {
	client, err := minio.New(c.MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(c.MinioAccessKey, c.MinioSecretKey, ""),
		Secure: c.MinioUseSSL,
	})
	if err != nil {
		return nil, err
	}

	return client, nil
}
