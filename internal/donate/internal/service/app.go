package service

import (
	donatev1 "github.com/lasthearth/vsservice/gen/donate/v1"
	"github.com/lasthearth/vsservice/internal/donate/internal/usecase"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mediaurl"
	"go.uber.org/fx"
)

var _ donatev1.DonateServiceServer = (*Service)(nil)

type Service struct {
	repo      DonateRepository
	purchases *usecase.Purchases
	log       logger.Logger
	mapper    Mapper
	mediaUrl  *mediaurl.Validator
}

type Opts struct {
	fx.In

	Repo      DonateRepository
	Purchases *usecase.Purchases
	Logger    logger.Logger
	Mapper    Mapper
	MediaURL  *mediaurl.Validator
}

func New(opts Opts) *Service {
	return &Service{
		repo:      opts.Repo,
		purchases: opts.Purchases,
		log:       opts.Logger,
		mapper:    opts.Mapper,
		mediaUrl:  opts.MediaURL,
	}
}
