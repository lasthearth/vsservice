package service

import (
	mailv1 "github.com/lasthearth/vsservice/gen/mail/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

var _ mailv1.MailServiceServer = (*Service)(nil)

type Service struct {
	repo   MailRepository
	log    logger.Logger
	mapper Mapper
	kits   KitReader
}

type Opts struct {
	fx.In

	Repo   MailRepository
	Logger logger.Logger
	Mapper Mapper
	Kits   KitReader
}

func New(opts Opts) *Service {
	return &Service{
		repo:   opts.Repo,
		log:    opts.Logger,
		mapper: opts.Mapper,
		kits:   opts.Kits,
	}
}
