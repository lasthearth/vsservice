package notification

import (
	"github.com/go-playground/validator/v10"
	notificationv1 "github.com/lasthearth/vsservice/gen/notification/v1"
	"github.com/lasthearth/vsservice/internal/notification/internal/repository"
	"github.com/lasthearth/vsservice/internal/notification/internal/repository/repomapper"
	"github.com/lasthearth/vsservice/internal/notification/internal/service"
	"github.com/lasthearth/vsservice/internal/notification/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/notification/usecase"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

const module = "notification"

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
			func() *validator.Validate {
				return validator.New(validator.WithRequiredStructEnabled())
			},
			fx.Annotate(
				func() *repomapper.NotificationMapperImpl {
					return &repomapper.NotificationMapperImpl{}
				},
				fx.As(new(repository.NotificationMapper)),
			),
			fx.Annotate(
				repository.New,
				fx.As(new(service.Repository)),
				fx.As(new(usecase.NotificationRepo)),
			),

			fx.Annotate(
				func() *sermapper.NotificationMapperImpl {
					return &sermapper.NotificationMapperImpl{}
				},
				fx.As(new(service.NotificationMapper)),
			),
		),

		fx.Provide(
			usecase.NewCreateNotificationUseCase,
		),

		fx.Provide(
			fx.Annotate(
				service.New,
				fx.As(new(notificationv1.NotificationServiceServer)),
			),
		),
	),
)
