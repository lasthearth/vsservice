package main

import (
	"context"
	"net/http"
	"time"

	"github.com/eapache/go-resiliency/retrier"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/lasthearth/vsservice/internal/leaderboard"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mongo"
	"github.com/lasthearth/vsservice/internal/rules"
	"github.com/lasthearth/vsservice/internal/server"
	"github.com/lasthearth/vsservice/internal/stats"
	"github.com/lasthearth/vsservice/internal/user"

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
			setupLogger,
			mongo.New,
			mongo.NewDatabase,
			// fx.Annotate(service.New, fx.As(new(vsservice.StatsService))),
		),

		leaderboard.App,
		stats.App,
		// trademarket.App,
		rules.App,
		server.App,
		user.App,
	)

	a.Run()

	defer func(app *fx.App, ctx context.Context) {
		err := app.Stop(ctx)
		panic(err)
	}(a, context.Background())

	<-a.Done()
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
