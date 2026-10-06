package event

import (
	eventv1 "github.com/lasthearth/vsservice/gen/event/v1"
	"github.com/lasthearth/vsservice/internal/event/internal/repository"
	"github.com/lasthearth/vsservice/internal/event/internal/repository/repomapper"
	"github.com/lasthearth/vsservice/internal/event/internal/service"
	"github.com/lasthearth/vsservice/internal/event/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

const module = "event"

// App is the events calendar module.
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
		),

		fx.Provide(
			fx.Annotate(
				service.New,
				fx.As(new(eventv1.EventServiceServer)),
			),
			fx.Annotate(
				service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
		),
	),
)
