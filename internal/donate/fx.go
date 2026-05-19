package donate

import (
	"context"

	donatev1 "github.com/lasthearth/vsservice/gen/donate/v1"
	repository "github.com/lasthearth/vsservice/internal/donate/internal/repository/mongo"
	"github.com/lasthearth/vsservice/internal/donate/internal/service"
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
				repository.New,
				fx.As(new(service.DonateRepository)),
			),
			fx.Annotate(
				pkgstorage.New,
				fx.As(new(service.Storage)),
			),
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

		fx.Invoke(func(lc fx.Lifecycle, s service.Storage) {
			lc.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					const bucket = "donate-shop"
					exists, err := s.BucketExists(ctx, bucket)
					if err != nil {
						return err
					}
					if exists {
						return nil
					}
					if err := s.CreateBucket(ctx, bucket); err != nil {
						return err
					}
					return s.MakeBucketPublic(ctx, bucket)
				},
			})
		}),
	),
)
