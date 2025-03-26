package leaderboard

import (
	v1 "github.com/ripls56/vsservice/gen/proto/v1"
	"github.com/ripls56/vsservice/internal/leaderboard/repository"
	"github.com/ripls56/vsservice/internal/leaderboard/service"
	"github.com/ripls56/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

const module = "leaderboard"

var App = fx.Options(
	fx.Module(
		module,
		fx.Decorate(
			func(l logger.Logger) logger.Logger {
				return l.WithScope(module)
			},
		),

		fx.Provide(
			fx.Private,
			fx.Annotate(
				repository.New,
				fx.As(new(service.Repository)),
			),
		),

		fx.Provide(
			fx.Annotate(service.New, fx.As(new(v1.LeaderboardServiceServer))),
		),
	),
)
