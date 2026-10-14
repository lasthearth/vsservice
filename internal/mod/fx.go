package mod

import (
	"net/http"

	modv1 "github.com/lasthearth/vsservice/gen/mod/v1"
	"github.com/lasthearth/vsservice/internal/mod/internal/repository"
	"github.com/lasthearth/vsservice/internal/mod/internal/repository/repomapper"
	"github.com/lasthearth/vsservice/internal/mod/internal/service"
	"github.com/lasthearth/vsservice/internal/mod/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/moddb"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

const module = "mod"

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
	),
)
