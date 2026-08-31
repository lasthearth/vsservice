package kitdef

import (
	kitdefv1 "github.com/lasthearth/vsservice/gen/kitdef/v1"
	repository "github.com/lasthearth/vsservice/internal/kitdef/internal/repository/mongo"
	"github.com/lasthearth/vsservice/internal/kitdef/internal/service"
	"github.com/lasthearth/vsservice/internal/kitdef/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/kitdef/kitdefread"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

const module = "kitdef"

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
				func() *sermapper.MapperImpl { return &sermapper.MapperImpl{} },
				fx.As(new(service.Mapper)),
			),
			fx.Annotate(
				repository.New,
				fx.As(new(service.KitRepository)),
			),
		),

		fx.Provide(
			fx.Annotate(service.New,
				fx.As(new(kitdefv1.KitDefServiceServer)),
			),
			fx.Annotate(service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
			// Public read port for other domains (mail's kit expansion). The
			// adapter maps the internal KitDef → kitdefread.KitDef so callers
			// never touch kitdef internals.
			fx.Annotate(
				func(opts service.Opts) kitdefread.Reader { return &reader{svc: service.New(opts)} },
			),
		),
	),
)
