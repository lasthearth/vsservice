package kit

import (
	kitv1 "github.com/lasthearth/vsservice/gen/kit/v1"
	repofx "github.com/lasthearth/vsservice/internal/kit/internal/repository/app"
	"github.com/lasthearth/vsservice/internal/kit/internal/service"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

var App = fx.Options(
	repofx.App,
	fx.Provide(
		fx.Annotate(service.NewFx,
			fx.As(new(kitv1.KitServiceServer)),
		),

		fx.Annotate(service.NewFx,
			fx.As(new(interceptor.Scoper)),
			fx.ResultTags(`group:"scopers"`),
		),
	),
)
