package reaction

import (
	reactionv1 "github.com/lasthearth/vsservice/gen/reaction/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/reaction/internal/repository"
	"github.com/lasthearth/vsservice/internal/reaction/internal/service"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

const module = "reaction"

// App is the emoji reactions module.
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
			fx.Annotate(
				service.New,
				fx.As(new(reactionv1.ReactionServiceServer)),
			),
			fx.Annotate(
				service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
		),
	),
)
