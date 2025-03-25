package main

import (
	"context"
	"github.com/ripls56/vsservice/internal/pkg/config"
	"github.com/ripls56/vsservice/internal/pkg/logger"
	"github.com/ripls56/vsservice/internal/pkg/mongo"
	"github.com/ripls56/vsservice/internal/server"
	vsservice "github.com/ripls56/vsservice/internal/service"
	"github.com/ripls56/vsservice/internal/stats/repository"
	service "github.com/ripls56/vsservice/internal/stats/service"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"net/http"
)

const (
	envDev  = "dev"
	envProd = "prod"
)

func main() {
	a := fx.New(
		fx.Provide(
			config.New,
			func() http.Client {
				return http.Client{
					Timeout: 5,
				}
			},
			setupLogger,
			mongo.New,
			mongo.NewDatabase,
			fx.Annotate(repository.New, fx.As(new(service.Repository))),
			fx.Annotate(service.New, fx.As(new(vsservice.StatsService))),
		),
		service.App,
		server.App,
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
