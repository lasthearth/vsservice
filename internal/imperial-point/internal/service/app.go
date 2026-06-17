package service

import (
	"sync"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In

	Log         logger.Logger
	Repo        ImperialPointRepository
	Progression ProgressionRollbacker
}

type Service struct {
	log         logger.Logger
	repo        ImperialPointRepository
	progression ProgressionRollbacker
	// mu serializes SetControl / ReleaseControl to prevent the count-check + write race
	// within a single process. Distributed deployments would need an external lock.
	mu sync.Mutex
}

func New(opts Opts) *Service {
	return &Service{
		log:         opts.Log,
		repo:        opts.Repo,
		progression: opts.Progression,
	}
}
