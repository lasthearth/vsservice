package appearance

import (
	appearancev1 "github.com/lasthearth/vsservice/gen/appearance/v1"
	"github.com/lasthearth/vsservice/internal/appearance/internal/repository"
	"github.com/lasthearth/vsservice/internal/appearance/internal/repository/repomapper"
	"github.com/lasthearth/vsservice/internal/appearance/internal/service"
	"github.com/lasthearth/vsservice/internal/appearance/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/donate/donateuc"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

const module = "appearance"

// App keeps how players look on the site: banners, avatar frames, their
// animations and titles.
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
				func() *repomapper.MapperImpl { return &repomapper.MapperImpl{} },
				fx.As(new(repository.Mapper)),
			),
			fx.Annotate(
				func() *sermapper.MapperImpl { return &sermapper.MapperImpl{} },
				fx.As(new(service.Mapper)),
			),
			fx.Annotate(
				repository.New,
				fx.As(new(service.Repository)),
			),
			fx.Annotate(
				repository.NewStandings,
				fx.As(new(service.StandingReader)),
			),
			// Shards are paid through donate's public use case.
			func(uc *donateuc.AddCoinsUseCase) service.Wallet { return uc },
		),

		fx.Provide(
			fx.Annotate(
				service.New,
				fx.As(new(appearancev1.AppearanceServiceServer)),
			),
			fx.Annotate(
				service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
		),
	),
)
