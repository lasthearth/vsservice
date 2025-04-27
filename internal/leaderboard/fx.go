package leaderboard

import (
	leaderboardv1 "github.com/lasthearth/vsservice/gen/leaderboard/v1"
	"github.com/lasthearth/vsservice/internal/leaderboard/internal/repository"
	service2 "github.com/lasthearth/vsservice/internal/leaderboard/internal/service"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
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
				fx.As(new(service2.Repository)),
			),
		),

		fx.Provide(
			fx.Annotate(service2.New, fx.As(new(leaderboardv1.LeaderboardServiceServer))),
		),
	),
)
