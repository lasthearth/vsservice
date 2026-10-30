package mod

import (
	"context"
	"fmt"
	"net/http"
	"time"

	modv1 "github.com/lasthearth/vsservice/gen/mod/v1"
	"github.com/lasthearth/vsservice/internal/mod/internal/repository"
	"github.com/lasthearth/vsservice/internal/mod/internal/repository/repomapper"
	"github.com/lasthearth/vsservice/internal/mod/internal/service"
	"github.com/lasthearth/vsservice/internal/mod/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/moddb"
	pkgstorage "github.com/lasthearth/vsservice/internal/pkg/storage"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	module = "mod"
	// defaultModsBucket repeats the config default for the bootstrap: a zero
	// config in a test must still name a bucket.
	defaultModsBucket = "server-mods"
	// bootstrapTimeout bounds the bucket bootstrap. A blackholed MinIO makes the
	// client retry past the fx start timeout, so the hook cannot wait on it for
	// the service to start; the sync retries the bucket on its own schedule.
	bootstrapTimeout = 5 * time.Second
)

// ArchiveHandler serves the required mods as one zip over plain HTTP. The
// gateway mounts it at /v1/mods/archive.zip, which is not a gRPC route: a zip
// is a file, and streaming it is what the site links to.
//
// It is an interface so the server mounts the handler without importing
// anything under this domain's internal tree.
type ArchiveHandler interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// App is the list of game-server mods shown on /mods.
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
			// The manifest source and the archive fallback both read objects
			// from MinIO; the startup hook creates the public bucket the game
			// server uploads to.
			fx.Annotate(
				pkgstorage.New,
				fx.As(new(service.ObjectStorage)),
				fx.As(new(service.BucketStore)),
			),
			moddb.New,
			// The service reads ModDB through a narrow interface; the archive
			// needs the client itself, because it streams a file rather than
			// reading a mod.
			fx.Annotate(
				func(client *moddb.Client) service.ModDB { return client },
				fx.As(new(service.ModDB)),
			),
			fx.Annotate(
				func() *repomapper.MapperImpl { return &repomapper.MapperImpl{} },
				fx.As(new(repository.Mapper)),
			),
			fx.Annotate(
				func() *sermapper.MapperImpl { return &sermapper.MapperImpl{} },
				fx.As(new(service.Mapper)),
			),
			fx.Annotate(
				repository.New,
				fx.As(new(service.Repository)),
				fx.As(new(service.RequiredMods)),
			),
			// One syncer for the whole module: every Service instance shares
			// the same cache, so the ticker and ListMods never read MinIO twice
			// for the same manifest.
			service.NewSyncer,
		),

		fx.Provide(
			fx.Annotate(
				service.NewArchive,
				fx.As(new(ArchiveHandler)),
			),
			fx.Annotate(
				service.New,
				fx.As(new(modv1.ModServiceServer)),
			),
			fx.Annotate(
				service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
		),

		fx.Invoke(func(
			lc fx.Lifecycle,
			syncer *service.Syncer,
			buckets service.BucketStore,
			cfg config.Config,
			log logger.Logger,
		) {
			lc.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					// Both steps are best effort and never fail the start: MinIO
					// or the game server may be down, and /mods must still serve
					// what Mongo holds. The bootstrap gets its own short bound.
					bootCtx, cancel := context.WithTimeout(ctx, bootstrapTimeout)
					defer cancel()

					if err := bootstrapModsBucket(bootCtx, buckets, cfg); err != nil {
						log.Warn("failed to bootstrap the server-mods bucket, serving the stored list",
							zap.Error(err))
					}

					go syncer.Run(ctx)

					return nil
				},
			})
		}),
	),
)

// bootstrapModsBucket creates the bucket the game server uploads the mods and
// the manifest to, and makes it public-read: the CDN serves those files straight
// to players, so a private bucket would leave every link broken.
func bootstrapModsBucket(ctx context.Context, buckets service.BucketStore, cfg config.Config) error {
	bucket := cfg.ModsBucket
	if bucket == "" {
		bucket = defaultModsBucket
	}

	exists, err := buckets.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("check the %s bucket: %w", bucket, err)
	}

	if !exists {
		if err := buckets.CreateBucket(ctx, bucket); err != nil {
			return fmt.Errorf("create the %s bucket: %w", bucket, err)
		}
	}

	if err := buckets.MakeBucketPublic(ctx, bucket); err != nil {
		return fmt.Errorf("make the %s bucket public: %w", bucket, err)
	}

	return nil
}
