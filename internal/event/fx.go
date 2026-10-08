package event

import (
	"context"

	eventv1 "github.com/lasthearth/vsservice/gen/event/v1"
	"github.com/lasthearth/vsservice/internal/event/internal/repository"
	"github.com/lasthearth/vsservice/internal/event/internal/repository/repomapper"
	"github.com/lasthearth/vsservice/internal/event/internal/service"
	"github.com/lasthearth/vsservice/internal/event/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

const module = "event"

// App is the events calendar module.
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
			),
		),

		fx.Provide(
			fx.Private,
			service.New,
		),

		fx.Provide(
			func(s *service.Service) eventv1.EventServiceServer { return s },
			fx.Annotate(
				func(s *service.Service) interceptor.Scoper { return s },
				fx.ResultTags(`group:"scopers"`),
			),
		),

		// Reminds attendees an hour before an event starts.
		fx.Invoke(func(lc fx.Lifecycle, s *service.Service) {
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			lc.Append(fx.Hook{
				OnStart: func(context.Context) error {
					go func() {
						defer close(done)
						s.RunReminders(ctx)
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
