package lfg

import (
	lfgv1 "github.com/lasthearth/vsservice/gen/lfg/v1"
	"github.com/lasthearth/vsservice/internal/lfg/internal/repository"
	"github.com/lasthearth/vsservice/internal/lfg/internal/repository/repomapper"
	"github.com/lasthearth/vsservice/internal/lfg/internal/service"
	"github.com/lasthearth/vsservice/internal/lfg/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

const module = "lfg"

// App is the "looking for company" board.
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
				fx.As(new(lfgv1.LfgServiceServer)),
			),
			fx.Annotate(
				service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
		),
	),
)
