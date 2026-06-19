package donate

import (
	donatev1 "github.com/lasthearth/vsservice/gen/donate/v1"
	"github.com/lasthearth/vsservice/internal/donate/donateuc"
	repository "github.com/lasthearth/vsservice/internal/donate/internal/repository/mongo"
	"github.com/lasthearth/vsservice/internal/donate/internal/repository/mongo/repomapper"
	"github.com/lasthearth/vsservice/internal/donate/internal/service"
	"github.com/lasthearth/vsservice/internal/donate/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	pkgstorage "github.com/lasthearth/vsservice/internal/pkg/storage"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

var module = "donate"

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
				func() *sermapper.MapperImpl {
					return &sermapper.MapperImpl{}
				},
				fx.As(new(service.Mapper)),
			),
			fx.Annotate(
				func() *repomapper.MapperImpl {
					return &repomapper.MapperImpl{}
				},
				fx.As(new(repository.Mapper)),
			),
			fx.Annotate(
				repository.New,
				fx.As(new(service.DonateRepository)),
				fx.As(new(donateuc.WalletRepo)),
			),
			fx.Annotate(
				pkgstorage.New,
				fx.As(new(service.Storage)),
			),
		),

		fx.Provide(
			donateuc.NewAddCoinsUseCase,
		),

		fx.Provide(
			fx.Annotate(service.New,
				fx.As(new(donatev1.DonateServiceServer)),
			),
			fx.Annotate(service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
		),
	),
)
