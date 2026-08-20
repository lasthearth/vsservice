package service

import (
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In

	Log   logger.Logger
	Repo  ProgressionRepository
	Favor FavorDeductor
}

type Service struct {
	log   logger.Logger
	repo  ProgressionRepository
	favor FavorDeductor
}

func New(opts Opts) *Service {
	return &Service{
		log:   opts.Log,
		repo:  opts.Repo,
		favor: opts.Favor,
	}
}
