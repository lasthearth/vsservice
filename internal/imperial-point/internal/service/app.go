package service

import (
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In

	Log  logger.Logger
	Repo ImperialPointRepository
}

type Service struct {
	log         logger.Logger
	repo        ImperialPointRepository
	progression ProgressionRollbacker
}

func New(opts Opts) *Service {
	return &Service{
		log:  opts.Log,
		repo: opts.Repo,
	}
}

func (s *Service) SetRollbacker(r ProgressionRollbacker) {
	s.progression = r
}
