package usecase

import (
	"context"

	"github.com/go-playground/validator/v10"
	"github.com/lasthearth/vsservice/internal/notification/internal/model"
	"github.com/lasthearth/vsservice/internal/notification/internal/repository"
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

type CreateNotification struct {
	repo     NotificationRepo
	validate *validator.Validate
}

func NewCreateNotificationUseCase(opts Opts) *CreateNotification {
	return &CreateNotification{
		repo:     opts.Repo,
		validate: opts.Validate,
	}
}

func (uc *CreateNotification) CreateNotification(ctx context.Context, notification model.Notification) error {
	if err := uc.validate.Struct(notification); err != nil {
		return err
	}

	return uc.repo.Create(ctx, notification)
}
