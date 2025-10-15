package verificationfx

import (
	verificationv1 "github.com/lasthearth/vsservice/gen/verification/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	prepository "github.com/lasthearth/vsservice/internal/player/internal/repository/player/repository"
	prepomapper "github.com/lasthearth/vsservice/internal/player/internal/repository/player/repository/repomapper"
	"github.com/lasthearth/vsservice/internal/player/internal/repository/verification/repository"
	"github.com/lasthearth/vsservice/internal/player/internal/repository/verification/repository/repomapper"
	"github.com/lasthearth/vsservice/internal/player/internal/repository/verification/ssorepository"
	service "github.com/lasthearth/vsservice/internal/player/internal/service/verification"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

const module = "verification"

var _ repository.Mapper = (*repomapper.MapperImpl)(nil)

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
				func() *prepomapper.MapperImpl {
					return &prepomapper.MapperImpl{}
				},
				fx.As(new(prepository.Mapper)),
			),

			fx.Annotate(
				func() *repomapper.MapperImpl {
					return &repomapper.MapperImpl{}
				},
				fx.As(new(repository.Mapper)),
			),
			fx.Annotate(
				repository.New,
				fx.As(new(service.DbRepository)),
			),

			fx.Annotate(
				ssorepository.New,
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
