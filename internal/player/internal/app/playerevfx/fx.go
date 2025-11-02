// Package playerevfx provides functionality for player-related events and effects.
package playerevfx

import (
	"context"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/player/internal/event"
	"github.com/lasthearth/vsservice/internal/player/internal/repository/player/repository"
	"github.com/lasthearth/vsservice/internal/player/internal/repository/player/repository/repomapper"
	"go.uber.org/fx"
)

const module = "player_events"

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
				repository.New,
				fx.As(new(event.PlayerRepository)),
			),

			event.NewEventManager,
		),

		fx.Invoke(
			func(lc fx.Lifecycle, log logger.Logger, ev *event.Bus) {
				lc.Append(
					fx.Hook{
						OnStart: func(ctx context.Context) error {
							ev.Subscribe()
							return nil
						},
						OnStop: func(ctx context.Context) error {
							ev.Unsubscribe()
							return nil
						},
					},
				)
			},
		),
	),
)
