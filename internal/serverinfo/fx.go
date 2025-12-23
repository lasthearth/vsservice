package serverinfo

import (
	serverinfov1 "github.com/lasthearth/vsservice/gen/serverinfo/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/serverinfo/internal/event"
	repofx "github.com/lasthearth/vsservice/internal/serverinfo/internal/repository/app"
	servicefx "github.com/lasthearth/vsservice/internal/serverinfo/internal/service/app"
	"go.uber.org/fx"
)

var module string = "serverinfo"

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
			event.NewEventManagerFx,
		),

		fx.Provide(
			fx.Annotate(servicefx.NewServiceFx,
				fx.As(new(serverinfov1.ServerInfoServiceServer)),
			),
		),

		fx.Invoke(
			func(
				lc fx.Lifecycle,
				bus *event.Bus,
			) {
				lc.Append(
					fx.StartStopHook(bus.Subscribe, bus.Unsubscribe),
				)
			},
		),
	),
)
