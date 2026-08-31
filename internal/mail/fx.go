package mail

import (
	mailv1 "github.com/lasthearth/vsservice/gen/mail/v1"
	repository "github.com/lasthearth/vsservice/internal/mail/internal/repository/mongo"
	"github.com/lasthearth/vsservice/internal/mail/internal/service"
	"github.com/lasthearth/vsservice/internal/mail/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/mail/mailcompose"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

const module = "mail"

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
				func() *sermapper.MapperImpl { return &sermapper.MapperImpl{} },
				fx.As(new(service.Mapper)),
			),
			fx.Annotate(
				repository.New,
				fx.As(new(service.MailRepository)),
			),
			// Adapt kitdef's public read port to mail's consumer KitReader at
			// the composition seam. mail imports kitdefread (public); kitdef
			// never imports mail.
			newKitReaderAdapter,
		),

		fx.Provide(
			fx.Annotate(service.New,
				fx.As(new(mailv1.MailServiceServer)),
			),
			fx.Annotate(service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
			// Outward composer port for other domains (donate).
			fx.Annotate(service.NewComposer,
				fx.As(new(mailcompose.MailComposer)),
			),
		),
	),
)
