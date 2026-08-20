package service

import (
	"github.com/go-playground/validator/v10"
	newsv1 "github.com/lasthearth/vsservice/gen/news/v1"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mediaurl"
	"go.uber.org/fx"
)

var _ newsv1.NewsServiceServer = (*Service)(nil)

type Opts struct {
	fx.In
	Logger               logger.Logger
	Repo                 Repository
	CreateNotificationUC *notificationuc.Create
	Mapper               Mapper
	Validator            *validator.Validate
	MediaURL             *mediaurl.Validator
}

type Service struct {
	logger logger.Logger
	repo   Repository
	// Create notification use case
	cnuc      *notificationuc.Create
	mapper    Mapper
	validator *validator.Validate
	mediaUrl  *mediaurl.Validator
}

func New(opts Opts) *Service {
	l := opts.Logger.WithComponent("service")
	return &Service{
		logger:    l,
		repo:      opts.Repo,
		cnuc:      opts.CreateNotificationUC,
		mapper:    opts.Mapper,
		validator: opts.Validator,
		mediaUrl:  opts.MediaURL,
	}
}
