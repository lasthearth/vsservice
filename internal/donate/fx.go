package donate

import (
	donatev1 "github.com/lasthearth/vsservice/gen/donate/v1"
	"github.com/lasthearth/vsservice/internal/donate/donateuc"
	repository "github.com/lasthearth/vsservice/internal/donate/internal/repository/mongo"
	"github.com/lasthearth/vsservice/internal/donate/internal/service"
	"github.com/lasthearth/vsservice/internal/donate/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/donate/internal/usecase"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
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
				repository.New,
				fx.As(new(service.DonateRepository)),
				fx.As(new(donateuc.WalletRepo)),
				fx.As(new(usecase.PurchaseRepo)),
				fx.As(new(usecase.Sequence)),
			),
		),

		fx.Provide(
			fx.Private,
			usecase.NewPurchases,
			// Adapt mail's public composer port to donate's consumer interface
			// at the composition seam. donate imports mailcompose (public); mail
			// never imports donate.
			newMailComposerAdapter,
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
