package media

import (
	"context"

	mediav1 "github.com/lasthearth/vsservice/gen/media/v1"
	"github.com/lasthearth/vsservice/internal/media/internal/service"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	pkgstorage "github.com/lasthearth/vsservice/internal/pkg/storage"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

var App = fx.Options(fx.Module("media",
	fx.Decorate(func(l logger.Logger) logger.Logger { return l.WithScope("media") }),
	fx.Provide(fx.Private,
		fx.Annotate(pkgstorage.New, fx.As(new(service.Storage))),
	),
	fx.Provide(
		fx.Annotate(service.New, fx.As(new(mediav1.MediaServiceServer))),
		fx.Annotate(service.New, fx.As(new(interceptor.Scoper)), fx.ResultTags(`group:"scopers"`)),
	),
	fx.Invoke(func(lc fx.Lifecycle, s service.Storage) {
		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				for _, bucket := range []string{"donate-shop", "settlementsreq", "news"} {
					exists, err := s.BucketExists(ctx, bucket)
					if err != nil {
						return err
					}
					if exists {
						continue
					}
					if err := s.CreateBucket(ctx, bucket); err != nil {
						return err
					}
					if err := s.MakeBucketPublic(ctx, bucket); err != nil {
						return err
					}
				}
				return nil
			},
		})
	}),
))
