package kit

import (
	kitv1 "github.com/lasthearth/vsservice/gen/kit/v1"
	repofx "github.com/lasthearth/vsservice/internal/kit/internal/repository/app"
	"github.com/lasthearth/vsservice/internal/kit/internal/service"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/nats-io/nats.go"
	"go.uber.org/fx"
)

var module string = "kit"

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

			func(
				nc *nats.Conn,
				l logger.Logger,
				assignmentRepo service.AssignmentRepository,
			) *service.Bus {
				return service.NewEventManager(nc, l, assignmentRepo)
			},
		),

		fx.Provide(
			fx.Annotate(service.NewFx,
				fx.As(new(kitv1.KitServiceServer)),
			),

			fx.Annotate(service.NewFx,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
		),

		fx.Invoke(
			func(
				lc fx.Lifecycle,
				bus *service.Bus,
			) {
				lc.Append(
					fx.StartStopHook(bus.Subscribe, bus.Unsubscribe),
				)
			},
		),
	),
)
