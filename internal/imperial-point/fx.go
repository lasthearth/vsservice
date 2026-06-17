package imperialpoint

import (
	imperialpointv1 "github.com/lasthearth/vsservice/gen/imperialpoint/v1"
	"github.com/lasthearth/vsservice/internal/imperial-point/internal/repository"
	"github.com/lasthearth/vsservice/internal/imperial-point/internal/service"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/pointcontrol"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

const module = "imperial-point"

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
				fx.As(new(service.ImperialPointRepository)),
			),
		),

		fx.Provide(
			fx.Annotate(
				service.New,
				fx.As(new(imperialpointv1.ImperialPointServiceServer)),
			),
			fx.Annotate(
				service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
			fx.Annotate(
				service.New,
				fx.As(new(pointcontrol.Reader)),
			),
		),
	),
)
