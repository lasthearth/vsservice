package user

import (
	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/user/internal/repository"
	"github.com/lasthearth/vsservice/internal/user/internal/service"
	"go.uber.org/fx"
)

const module = "users"

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
			fx.Annotate(service.New,
				fx.As(new(userv1.UserServiceServer)),
			),

			// fx.Annotate(service.New,
			// 	fx.As(new(interceptor.Scoper)),
			// 	fx.ResultTags(`group:"scopers"`),
			// ),
		),
	),
)
