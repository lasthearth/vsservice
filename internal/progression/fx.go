package progression

import (
	progressionv1 "github.com/lasthearth/vsservice/gen/progression/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/pointcontrol"
	"github.com/lasthearth/vsservice/internal/progression/internal/repository"
	"github.com/lasthearth/vsservice/internal/progression/internal/service"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/settlement/settlementuc"
	"go.uber.org/fx"
)

const module = "progression"

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
				fx.As(new(service.ProgressionRepository)),
			),
			fx.Annotate(
				func(f *settlementuc.FavorOps) service.FavorDeductor { return f },
			),
		),

		// Single *Service instance shared across all role bindings.
		fx.Provide(service.New),

		fx.Provide(
			fx.Annotate(
				func(s *service.Service) progressionv1.ProgressionServiceServer { return s },
			),
			fx.Annotate(
				func(s *service.Service) interceptor.Scoper { return s },
				fx.ResultTags(`group:"scopers"`),
			),
			fx.Annotate(
				func(s *service.Service) pointcontrol.Rollbacker { return s },
			),
		),
	),
)
