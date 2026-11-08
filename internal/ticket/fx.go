package ticket

import (
	"context"

	ticketv1 "github.com/lasthearth/vsservice/gen/ticket/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/ticket/internal/notify"
	"github.com/lasthearth/vsservice/internal/ticket/internal/repository"
	"github.com/lasthearth/vsservice/internal/ticket/internal/repository/repomapper"
	"github.com/lasthearth/vsservice/internal/ticket/internal/service"
	"go.uber.org/fx"
)

const module = "ticket"

// App is the support desk: player tickets and the staff inbox.
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
				repository.New,
				fx.As(new(service.Repository)),
			),
			fx.Annotate(
				notify.New,
				fx.As(new(service.Notifier)),
			),
			service.New,
		),

		fx.Provide(
			func(s *service.Service) ticketv1.TicketServiceServer { return s },
			fx.Annotate(
				func(s *service.Service) interceptor.Scoper { return s },
				fx.ResultTags(`group:"scopers"`),
			),
		),

		// Closes the answered tickets players left quiet for a week.
		fx.Invoke(func(lc fx.Lifecycle, s *service.Service) {
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			lc.Append(fx.Hook{
				OnStart: func(context.Context) error {
					go func() {
						defer close(done)
						s.RunAutoClose(ctx)
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
