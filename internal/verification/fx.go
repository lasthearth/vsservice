package verification

import (
	verificationv1 "github.com/lasthearth/vsservice/gen/verification/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	mongorepo "github.com/lasthearth/vsservice/internal/verification/internal/repository/mongo"
	ssorepo "github.com/lasthearth/vsservice/internal/verification/internal/repository/sso"
	"github.com/lasthearth/vsservice/internal/verification/internal/service"
	"go.uber.org/fx"
)

const module = "verification"

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
				fx.As(new(service.VerificationDbRepository)),
			),

			fx.Annotate(
				ssorepo.New,
				fx.As(new(service.SsoRepository)),
			),
		),

		fx.Provide(
			fx.Annotate(service.New,
				fx.As(new(verificationv1.VerificationServiceServer)),
			),

			fx.Annotate(service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
		),
	),
)
