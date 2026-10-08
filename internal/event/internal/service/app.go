package service

import (
	"context"
	"time"

	eventv1 "github.com/lasthearth/vsservice/gen/event/v1"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mediaurl"
	"go.uber.org/fx"
)

var _ eventv1.EventServiceServer = (*Service)(nil)

type Opts struct {
	fx.In
	Logger               logger.Logger
	Repo                 Repository
	Mapper               Mapper
	CreateNotificationUC *notificationuc.Create
	MediaURL             *mediaurl.Validator
}

// notifier sends a notification; *notificationuc.Create in production.
type notifier interface {
	CreateNotification(ctx context.Context, title, message string, opts ...notificationuc.NotificationOpts) error
}

type Service struct {
	logger   logger.Logger
	repo     Repository
	mapper   Mapper
	cnuc     notifier
	mediaURL *mediaurl.Validator
	now      func() time.Time
}

func New(opts Opts) *Service {
	s := &Service{
		logger:   opts.Logger.WithComponent("service"),
		repo:     opts.Repo,
		mapper:   opts.Mapper,
		mediaURL: opts.MediaURL,
		now:      time.Now,
	}
	// A nil *Create stored in the interface would not compare equal to nil.
	if opts.CreateNotificationUC != nil {
		s.cnuc = opts.CreateNotificationUC
	}
	return s
}
