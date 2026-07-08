package discord

import (
	discordv1 "github.com/lasthearth/vsservice/gen/discord/v1"
	"github.com/lasthearth/vsservice/internal/discord/internal/discord"
	"github.com/lasthearth/vsservice/internal/discord/internal/service"
	"github.com/lasthearth/vsservice/internal/discord/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

const module = "discord"

// App exports the discord domain module.
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
				func() *sermapper.MapperImpl {
					return &sermapper.MapperImpl{}
				},
				fx.As(new(service.Mapper)),
			),
			fx.Annotate(
				discord.NewClient,
				fx.As(new(service.DiscordClient)),
			),
		),

		fx.Provide(
			fx.Annotate(
				service.New,
				fx.As(new(discordv1.DiscordServiceServer)),
			),
			fx.Annotate(
				service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
		),
	),
)
