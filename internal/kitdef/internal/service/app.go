package service

import (
	kitdefv1 "github.com/lasthearth/vsservice/gen/kitdef/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

var _ kitdefv1.KitDefServiceServer = (*Service)(nil)

type Service struct {
	repo   KitRepository
	log    logger.Logger
	mapper Mapper
}

type Opts struct {
	fx.In

	Repo   KitRepository
	Logger logger.Logger
	Mapper Mapper
}

func New(opts Opts) *Service {
	return &Service{
		repo:   opts.Repo,
		log:    opts.Logger,
		mapper: opts.Mapper,
	}
}
