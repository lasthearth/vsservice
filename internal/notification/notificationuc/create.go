package notificationuc

import (
	"context"
	"time"

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

type NotificationOpts func(*model.Notification)

// CreateNotification creates a new notification, for user.
//
// For sending broadcast notifications specify userId with model.BroadcastUserId
func (uc *Create) CreateNotification(
	ctx context.Context,
	title,
	message string,
	opts ...NotificationOpts,
) error {
	now := time.Now()
	notification := model.Notification{
		Title:     title,
		Message:   message,
		State:     model.NotificationStateUnread,
		CreatedAt: now,
		UpdatedAt: now,
	}

	for _, opt := range opts {
		opt(&notification)
	}

	if err := uc.validate.Struct(notification); err != nil {
		return err
	}

	return uc.repo.Create(ctx, notification)
}

func WithUserId(userId string) NotificationOpts {
	return func(n *model.Notification) {
		n.UserId = userId
	}
}

func WithBroadcast() NotificationOpts {
	return func(n *model.Notification) {
		n.UserId = model.BroadcastUserId
	}
}
