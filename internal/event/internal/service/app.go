package service

import (
	"context"
	"time"

	eventv1 "github.com/lasthearth/vsservice/gen/event/v1"
	"github.com/lasthearth/vsservice/internal/event/internal/model"
	"github.com/lasthearth/vsservice/internal/event/internal/repository"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mediaurl"
	"go.uber.org/fx"
)

var _ eventv1.EventServiceServer = (*Service)(nil)

var _ Repository = (*repository.Repository)(nil)

// Repository stores events.
type Repository interface {
	Create(ctx context.Context, event *model.Event) (*model.Event, error)
	Get(ctx context.Context, id string) (*model.Event, error)
	Update(ctx context.Context, event *model.Event) (*model.Event, error)
	SoftDelete(ctx context.Context, id, deletedBy string) error
	ListUpcoming(ctx context.Context, now time.Time, limit int) ([]model.Event, error)
	ListPast(ctx context.Context, now time.Time, limit int) ([]model.Event, error)
}

type Opts struct {
	fx.In
	Logger               logger.Logger
	Repo                 Repository
	CreateNotificationUC *notificationuc.Create
	MediaURL             *mediaurl.Validator
}

type Service struct {
	logger   logger.Logger
	repo     Repository
	cnuc     *notificationuc.Create
	mediaURL *mediaurl.Validator
	now      func() time.Time
}

func New(opts Opts) *Service {
	return &Service{
		logger:   opts.Logger.WithComponent("service"),
		repo:     opts.Repo,
		cnuc:     opts.CreateNotificationUC,
		mediaURL: opts.MediaURL,
		now:      time.Now,
	}
}
