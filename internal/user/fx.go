package user

import (
	"context"

	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/storage"
	mongorepo "github.com/lasthearth/vsservice/internal/user/internal/repository/mongo"
	"github.com/lasthearth/vsservice/internal/user/internal/repository/sso"
	"github.com/lasthearth/vsservice/internal/user/internal/service"
	"go.uber.org/fx"
)

const module = "users"

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
				storage.New,
				fx.As(new(service.Storage)),
			),
			fx.Annotate(
				sso.New,
				fx.As(new(service.SsoRepository)),
			),
			fx.Annotate(
				mongorepo.New,
				fx.As(new(service.DbRepository)),
			),
		),

		fx.Provide(
			fx.Annotate(service.New,
				fx.As(new(userv1.UserServiceServer)),
			),

			// fx.Annotate(service.New,
			// 	fx.As(new(interceptor.Scoper)),
			// 	fx.ResultTags(`group:"scopers"`),
			// ),
		),

		fx.Invoke(func(lc fx.Lifecycle, storage service.Storage) {
			lc.Append(fx.Hook{
				OnStart: func(context.Context) error {
					exists, err := storage.BucketExists(context.Background(), "avatars")
					if err != nil {
						return err
					}
					if exists {
						return nil
					}

					err = storage.CreateBucket(context.Background(), "avatars")
					if err != nil {
						return err
					}

					err = storage.MakeBucketPublic(context.Background(), "avatars")
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
