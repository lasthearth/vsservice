package service

import (
	"time"

	appearancev1 "github.com/lasthearth/vsservice/gen/appearance/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

var _ appearancev1.AppearanceServiceServer = (*Service)(nil)

type Opts struct {
	fx.In
	Logger    logger.Logger
	Repo      Repository
	Standings StandingReader
	Wallet    Wallet
	Mapper    Mapper
}

type Service struct {
	logger    logger.Logger
	repo      Repository
	standings StandingReader
	wallet    Wallet
	mapper    Mapper
	now       func() time.Time
}

func New(opts Opts) *Service {
	return &Service{
		logger:    opts.Logger.WithComponent("service"),
		repo:      opts.Repo,
		standings: opts.Standings,
		wallet:    opts.Wallet,
		mapper:    opts.Mapper,
		now:       time.Now,
	}
}
