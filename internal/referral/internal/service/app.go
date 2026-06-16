package service

import (
	referralv1 "github.com/lasthearth/vsservice/gen/referral/v1"
	"github.com/lasthearth/vsservice/internal/donate/donateuc"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

var _ referralv1.ReferralServiceServer = (*Service)(nil)

type Opts struct {
	fx.In
	Log      logger.Logger
	Cfg      config.Config
	DbRepo   Repository
	DonateUC *donateuc.AddCoinsUseCase
}

type Service struct {
	log      logger.Logger
	cfg      config.Config
	dbRepo   Repository
	donateUC *donateuc.AddCoinsUseCase
}

func New(opts Opts) *Service {
	return &Service{
		log:      opts.Log,
		cfg:      opts.Cfg,
		dbRepo:   opts.DbRepo,
		donateUC: opts.DonateUC,
	}
}
