package service

import (
	"github.com/go-playground/validator/v10"
	newsv1 "github.com/lasthearth/vsservice/gen/news/v1"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

var _ newsv1.NewsServiceServer = (*Service)(nil)

type Opts struct {
	fx.In
	Logger               logger.Logger
	Repo                 Repository
	CreateNotificationUC *notificationuc.Create
	Storage              Storage
	Mapper               Mapper
	Config               config.Config
	Validator            *validator.Validate
}

type Service struct {
	logger logger.Logger
	repo   Repository
	// Create notification use case
	cnuc      *notificationuc.Create
	storage   Storage
	mapper    Mapper
	config    config.Config
	validator *validator.Validate
}

func New(opts Opts) *Service {
	l := opts.Logger.WithComponent("service")
	return &Service{
		logger:    l,
		repo:      opts.Repo,
		cnuc:      opts.CreateNotificationUC,
		storage:   opts.Storage,
		mapper:    opts.Mapper,
		config:    opts.Config,
		validator: opts.Validator,
	}
}
