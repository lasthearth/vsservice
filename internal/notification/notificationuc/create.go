package notificationuc

import (
	"context"

	"github.com/go-playground/validator/v10"
	"github.com/lasthearth/vsservice/internal/notification/internal/repository"
	"github.com/lasthearth/vsservice/internal/notification/model"
	"go.uber.org/fx"
)

var _ NotificationRepo = (*repository.Repository)(nil)

type NotificationRepo interface {
	Create(context.Context, model.Notification) error
}

type Opts struct {
	fx.In
	Repo     NotificationRepo
	Validate *validator.Validate
}

type Create struct {
	repo     NotificationRepo
	validate *validator.Validate
}

func NewCreateNotificationUseCase(opts Opts) *Create {
	return &Create{
		repo:     opts.Repo,
		validate: opts.Validate,
	}
}

func (uc *Create) CreateNotification(ctx context.Context, notification model.Notification) error {
	if err := uc.validate.Struct(notification); err != nil {
		return err
	}

	return uc.repo.Create(ctx, notification)
}
