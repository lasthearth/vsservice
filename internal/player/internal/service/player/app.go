package service

import (
	"context"

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

// notifier sends one site notification; *notificationuc.Create in production.
// The interface is spelled with the use case's own signature so a test can
// stand in for it without the notification's internal model, which no other
// domain may import.
type notifier interface {
	CreateNotification(ctx context.Context, title, message string, opts ...notificationuc.NotificationOpts) error
}

type Service struct {
	dbRepo  DbRepository
	ssoRepo SsoRepository
	storage Storage
	cfg     config.Config
	mapper  Mapper
	log     logger.Logger
	cnuc    notifier
}

func New(opts Opts) *Service {
	s := &Service{
		dbRepo:  opts.DbRepo,
		storage: opts.Storage,
		ssoRepo: opts.SsoRepo,
		cfg:     opts.Cfg,
		mapper:  opts.Mapper,
		log:     opts.Logger,
	}
	// A nil *Create stored in the interface would not compare equal to nil.
	if opts.CreateNotificationUC != nil {
		s.cnuc = opts.CreateNotificationUC
	}
	return s
}
