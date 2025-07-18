package playerfx

import (
	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/storage"
	"github.com/lasthearth/vsservice/internal/player/internal/repository/player/repository"
	"github.com/lasthearth/vsservice/internal/player/internal/repository/player/repository/repomapper"
	"github.com/lasthearth/vsservice/internal/player/internal/repository/player/sso"
	service "github.com/lasthearth/vsservice/internal/player/internal/service/player"
	"go.uber.org/fx"
)

const module = "player"

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
				storage.New,
				fx.As(new(service.Storage)),
			),

			fx.Annotate(
				func() *repomapper.MapperImpl {
					return &repomapper.MapperImpl{}
				},
				fx.As(new(repository.Mapper)),
			),
			fx.Annotate(
				repository.New,
				fx.As(new(service.DbRepository)),
			),

			fx.Annotate(
				sso.New,
				fx.As(new(service.SsoRepository)),
			),
		),

		fx.Provide(
			fx.Annotate(service.New,
				fx.As(new(userv1.UserServiceServer)),
			),
		),
	),
)
