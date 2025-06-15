package settlement

import (
	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	mongorepo "github.com/lasthearth/vsservice/internal/settlement/internal/repository/mongo"
	repository "github.com/lasthearth/vsservice/internal/settlement/internal/repository/mongo"
	"github.com/lasthearth/vsservice/internal/settlement/internal/repository/mongo/repomapper"
	"github.com/lasthearth/vsservice/internal/settlement/internal/service"
	"github.com/lasthearth/vsservice/internal/settlement/internal/service/sermapper"
	"go.uber.org/fx"
)

const module = "settlement"

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
				func() *sermapper.MapperImpl {
					return &sermapper.MapperImpl{}
				},
				fx.As(new(service.Mapper)),
			),
			fx.Annotate(
				mongorepo.New,
				fx.As(new(service.SettlementRepository)),
			),
		),

		fx.Provide(
			fx.Annotate(service.New,
				fx.As(new(settlementv1.SettlementServiceServer)),
			),

			fx.Annotate(service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
		),
	),
)
