package service

import (
	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

var _ userv1.UserServiceServer = (*Service)(nil)

type Opts struct {
	fx.In
	DbRepo               DbRepository
	SsoRepo              SsoRepository
	Storage              Storage
	Cfg                  config.Config
	Mapper               Mapper
	Logger               logger.Logger
	CreateNotificationUC *notificationuc.Create
}

type Service struct {
	dbRepo  DbRepository
	ssoRepo SsoRepository
	storage Storage
	cfg     config.Config
	mapper  Mapper
	log     logger.Logger
	cnuc    *notificationuc.Create
}

func New(opts Opts) *Service {
	return &Service{
		dbRepo:  opts.DbRepo,
		storage: opts.Storage,
		ssoRepo: opts.SsoRepo,
		cfg:     opts.Cfg,
		mapper:  opts.Mapper,
		log:     opts.Logger,
		cnuc:    opts.CreateNotificationUC,
	}
}
