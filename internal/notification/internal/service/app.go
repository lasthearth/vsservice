package service

import (
	notificationv1 "github.com/lasthearth/vsservice/gen/notification/v1"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

var _ notificationv1.NotificationServiceServer = (*Service)(nil)

type Opts struct {
	fx.In
	Log      logger.Logger
	Mapper   NotificationMapper
	Repo     Repository
	CreateUC *notificationuc.Create
}
type Service struct {
	log      logger.Logger
	mapper   NotificationMapper
	repo     Repository
	createUC *notificationuc.Create
}

func New(opts Opts) *Service {
	return &Service{
		log:      opts.Log,
		mapper:   opts.Mapper,
		repo:     opts.Repo,
		createUC: opts.CreateUC,
	}
}
