package service

import (
	"time"

	lfgv1 "github.com/lasthearth/vsservice/gen/lfg/v1"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

var _ lfgv1.LfgServiceServer = (*Service)(nil)

type Opts struct {
	fx.In
	Logger               logger.Logger
	Repo                 Repository
	Mapper               Mapper
	CreateNotificationUC *notificationuc.Create
}

type Service struct {
	logger logger.Logger
	repo   Repository
	mapper Mapper
	cnuc   *notificationuc.Create
	now    func() time.Time
}

func New(opts Opts) *Service {
	return &Service{
		logger: opts.Logger.WithComponent("service"),
		repo:   opts.Repo,
		mapper: opts.Mapper,
		cnuc:   opts.CreateNotificationUC,
		now:    time.Now,
	}
}
