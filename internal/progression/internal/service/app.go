package service

import (
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/pointcontrol"
	"github.com/lasthearth/vsservice/internal/settlement/settlementuc"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In

	Log       logger.Logger
	Repo      ProgressionRepository
	Favor     *settlementuc.FavorOps
	PointCtrl pointcontrol.Reader
}

type Service struct {
	log       logger.Logger
	repo      ProgressionRepository
	favor     *settlementuc.FavorOps
	pointCtrl pointcontrol.Reader
}

func New(opts Opts) *Service {
	return &Service{
		log:       opts.Log,
		repo:      opts.Repo,
		favor:     opts.Favor,
		pointCtrl: opts.PointCtrl,
	}
}
