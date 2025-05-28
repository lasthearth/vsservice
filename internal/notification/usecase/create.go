package usecase

import (
	"context"

	"github.com/go-playground/validator/v10"
	"github.com/lasthearth/vsservice/internal/notification/internal/model"
	"github.com/lasthearth/vsservice/internal/notification/internal/repository"
)

var _ NotificationRepo = (*repository.Repository)(nil)

type NotificationRepo interface {
	Create(context.Context, model.Notification) error
}

type Opts struct {
	Repo     repository.Repository
	Validate *validator.Validate
}

type CreateNotificationUseCase struct {
	repo     repository.Repository
	validate *validator.Validate
}

func NewCreateNotificationUseCase(opts Opts) *CreateNotificationUseCase {
	return &CreateNotificationUseCase{
		repo:     opts.Repo,
		validate: opts.Validate,
	}
}

func (uc *CreateNotificationUseCase) CreateNotification(ctx context.Context, notification model.Notification) error {
	if err := uc.validate.Struct(notification); err != nil {
		return err
	}

	return uc.repo.Create(ctx, notification)
}
