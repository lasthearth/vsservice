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

		// Single *Service instance shared across all role bindings.
		fx.Provide(service.New),

		fx.Provide(
			fx.Annotate(
				func(s *service.Service) imperialpointv1.ImperialPointServiceServer { return s },
			),
			fx.Annotate(
				func(s *service.Service) interceptor.Scoper { return s },
				fx.ResultTags(`group:"scopers"`),
			),
			fx.Annotate(
				func(s *service.Service) pointcontrol.Reader { return s },
			),
		),
	),
)
