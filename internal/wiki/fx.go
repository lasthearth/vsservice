// Package wiki wires the wiki domain into the application.
package wiki

import (
	wikiv1 "github.com/lasthearth/vsservice/gen/wiki/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/wiki/internal/repository"
	"github.com/lasthearth/vsservice/internal/wiki/internal/repository/repomapper"
	"github.com/lasthearth/vsservice/internal/wiki/internal/service"
	"github.com/lasthearth/vsservice/internal/wiki/internal/service/sermapper"
	"go.uber.org/fx"
)

const module = "wiki"

// App is the wiki domain module.
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
				func() *repomapper.MapperImpl {
					return &repomapper.MapperImpl{}
				},
				fx.As(new(repository.Mapper)),
			),
			fx.Annotate(
				repository.New,
				fx.As(new(service.Repository)),
			),
			fx.Annotate(
				func() *sermapper.MapperImpl {
					return &sermapper.MapperImpl{}
				},
				fx.As(new(service.Mapper)),
			),
		),

		fx.Provide(
			// NewNotifier already returns the interface the service depends on.
			service.NewNotifier,
			fx.Annotate(
				service.New,
				fx.As(new(wikiv1.WikiServiceServer)),
			),
			fx.Annotate(
				service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
		),
	),
)
