package service

import (
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

type Service struct {
	logger   logger.Logger
	repo     Repository
	mapper   Mapper
	cnuc     *notificationuc.Create
	mediaURL *mediaurl.Validator
	now      func() time.Time
}

func New(opts Opts) *Service {
	return &Service{
		logger:   opts.Logger.WithComponent("service"),
		repo:     opts.Repo,
		mapper:   opts.Mapper,
		cnuc:     opts.CreateNotificationUC,
		mediaURL: opts.MediaURL,
		now:      time.Now,
	}
}
