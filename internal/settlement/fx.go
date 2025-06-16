package settlement

import (
	"context"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/storage"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
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
				storage.New,
				fx.As(new(service.Storage)),
			),
			fx.Annotate(
				repository.New,
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

		fx.Invoke(func(lc fx.Lifecycle, storage service.Storage) {
			lc.Append(fx.Hook{
				OnStart: func(context.Context) error {
					bucketName := "settlementreq"
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
