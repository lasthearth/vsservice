package referral

import (
	referralv1 "github.com/lasthearth/vsservice/gen/referral/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	mongorepo "github.com/lasthearth/vsservice/internal/referral/internal/repository/referral"
	"github.com/lasthearth/vsservice/internal/referral/internal/service"
	"go.uber.org/fx"
)

const module = "referral"

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
				mongorepo.New,
				fx.As(new(service.Repository)),
			),
		),

		fx.Provide(
			fx.Annotate(service.New,
				fx.As(new(referralv1.ReferralServiceServer)),
			),
		),
	),
)
