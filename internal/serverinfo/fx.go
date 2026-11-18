package serverinfo

import (
	"context"
	"net/http"

	serverinfov1 "github.com/lasthearth/vsservice/gen/serverinfo/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	repofx "github.com/lasthearth/vsservice/internal/serverinfo/internal/repository/app"
	servicefx "github.com/lasthearth/vsservice/internal/serverinfo/internal/service/app"
	"github.com/lasthearth/vsservice/internal/serverinfo/internal/service/serverinfo"
	"go.uber.org/fx"
)

var module = "serverinfo"

// StreamHandler is the live serverinfo feed: an SSE route the gateway mounts at
// /v1/serverinfo/stream, which is not a gRPC route because an event stream is a
// connection, not a message.
//
// Run starts the single poller the feed needs, so the lifecycle hook that owns
// it works through the same value the gateway mounts. It is an interface so the
// server mounts the handler without importing anything under this domain's
// internal tree.
type StreamHandler interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request)
	Run(ctx context.Context)
}

var App = fx.Options(
	fx.Module(
		module,
		fx.Decorate(
			func(l logger.Logger) logger.Logger {
				return l.WithScope(module)
			},
		),
		repofx.App,

		fx.Provide(
			fx.Private,
			servicefx.NewSweeperFx,
		),

		fx.Provide(
			fx.Annotate(servicefx.NewServiceFx,
				fx.As(new(serverinfov1.ServerInfoServiceServer)),
			),
			// One Stream per process: the poller behind it is the only reader of
			// the document on the feed's behalf, and a second value would mean a
			// second reader.
			fx.Annotate(serverinfo.NewStream,
				fx.As(new(StreamHandler)),
			),
		),

		// Zeroes a total_online whose heartbeat went stale: the game server owns
		// the document and stops writing when it dies.
		fx.Invoke(func(lc fx.Lifecycle, s *serverinfo.Sweeper) {
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			lc.Append(fx.Hook{
				OnStart: func(context.Context) error {
					go func() {
						defer close(done)
						s.Run(ctx)
					}()
					return nil
				},
				OnStop: func(stopCtx context.Context) error {
					cancel()
					select {
					case <-done:
					case <-stopCtx.Done():
					}
					return nil
				},
			})
		}),

		// The feed's single poller. Its lifetime is the app's, not the start
		// hook's: the hook context carries the fx StartTimeout and is cancelled
		// about 15 seconds after start, which would stop the feed silently while
		// the gateway kept accepting subscribers.
		fx.Invoke(func(lc fx.Lifecycle, s StreamHandler) {
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			lc.Append(fx.Hook{
				OnStart: func(context.Context) error {
					go func() {
						defer close(done)
						s.Run(ctx)
					}()
					return nil
				},
				OnStop: func(stopCtx context.Context) error {
					cancel()
					select {
					case <-done:
					case <-stopCtx.Done():
					}
					return nil
				},
			})
		}),
	),
)
