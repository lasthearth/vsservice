package service

import (
	donatev1 "github.com/lasthearth/vsservice/gen/donate/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

var _ donatev1.DonateServiceServer = (*Service)(nil)

const bucketName = "donate-shop"

type Service struct {
	repo    DonateRepository
	storage Storage
	cfg     config.Config
	log     logger.Logger
	mapper  Mapper
}

type Opts struct {
	fx.In

	Repo    DonateRepository
	Storage Storage
	Config  config.Config
	Logger  logger.Logger
}

func New(opts Opts) *Service {
	return &Service{
		repo:    opts.Repo,
		storage: opts.Storage,
		cfg:     opts.Config,
		log:     opts.Logger,
		mapper:  &MapperImpl{},
	}
}
