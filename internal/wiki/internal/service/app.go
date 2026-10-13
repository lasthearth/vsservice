package service

import (
	wikiv1 "github.com/lasthearth/vsservice/gen/wiki/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

var _ wikiv1.WikiServiceServer = (*Service)(nil)

// Opts is what the wiki service needs from the container.
type Opts struct {
	fx.In
	Logger   logger.Logger
	Repo     Repository
	Mapper   Mapper
	Notifier Notifier
}

// Service implements the wiki API.
type Service struct {
	logger   logger.Logger
	repo     Repository
	mapper   Mapper
	notifier Notifier
}

// New builds the wiki service.
func New(opts Opts) *Service {
	return &Service{
		logger:   opts.Logger.WithComponent("service"),
		repo:     opts.Repo,
		mapper:   opts.Mapper,
		notifier: opts.Notifier,
	}
}
