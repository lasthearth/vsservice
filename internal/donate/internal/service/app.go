package service

import (
	donatev1 "github.com/lasthearth/vsservice/gen/donate/v1"
	"github.com/lasthearth/vsservice/internal/donate/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mediaurl"
	"go.uber.org/fx"
)

var _ donatev1.DonateServiceServer = (*Service)(nil)

type Service struct {
	repo     DonateRepository
	storage  Storage
	cfg      config.Config
	log      logger.Logger
	mapper   Mapper
	mediaUrl *mediaurl.Validator
}

type Opts struct {
	fx.In

	Repo     DonateRepository
	Storage  Storage
	Config   config.Config
	Logger   logger.Logger
	MediaURL *mediaurl.Validator
}

func New(opts Opts) *Service {
	return &Service{
		repo:     opts.Repo,
		storage:  opts.Storage,
		cfg:      opts.Config,
		log:      opts.Logger,
		mapper:   &sermapper.MapperImpl{},
		mediaUrl: opts.MediaURL,
	}
}
