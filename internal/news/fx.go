package news

import (
	"context"

	"github.com/go-playground/validator/v10"
	newsv1 "github.com/lasthearth/vsservice/gen/news/v1"
	"github.com/lasthearth/vsservice/internal/news/internal/repository"
	"github.com/lasthearth/vsservice/internal/news/internal/repository/repomapper"
	"github.com/lasthearth/vsservice/internal/news/internal/service"
	"github.com/lasthearth/vsservice/internal/news/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/storage"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

const module = "news"

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
			func() *validator.Validate {
				return validator.New(validator.WithRequiredStructEnabled())
			},
			fx.Annotate(
				func() *repomapper.MapperImpl {
					return &repomapper.MapperImpl{}
				},
				fx.As(new(repository.Mapper)),
			),
			fx.Annotate(
				storage.New,
				fx.As(new(service.Storage)),
			),
			fx.Annotate(
				repository.New,
				fx.As(new(service.Repository)),
			),

			fx.Annotate(
				func() *sermapper.MapperImpl {
					return &sermapper.MapperImpl{}
				},
				fx.As(new(service.Mapper)),
			),
		),

		fx.Provide(
			fx.Annotate(
				service.New,
				fx.As(new(newsv1.NewsServiceServer)),
			),
			fx.Annotate(
				service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
		),

		fx.Invoke(func(lc fx.Lifecycle, storage service.Storage) {
			lc.Append(fx.Hook{
				OnStart: func(context.Context) error {
					bucketName := "news"
					exists, err := storage.BucketExists(context.Background(), bucketName)
					if err != nil {
						return err
					}
					if exists {
						return nil
					}

					err = storage.CreateBucket(context.Background(), bucketName)
					if err != nil {
						return err
					}

					err = storage.MakeBucketPublic(context.Background(), bucketName)
					if err != nil {
						return err
					}
					return nil
				},
				OnStop: nil,
			})
		}),
	),
)
