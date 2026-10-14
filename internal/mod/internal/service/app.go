package service

import (
	"time"

	modv1 "github.com/lasthearth/vsservice/gen/mod/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

var _ modv1.ModServiceServer = (*Service)(nil)

type Opts struct {
	fx.In
	Logger logger.Logger
	Repo   Repository
	Mapper Mapper
	ModDB  ModDB
}

type Service struct {
	log    logger.Logger
	repo   Repository
	mapper Mapper
	moddb  ModDB
	// now is a field so a test can pin the clock the archive timestamp and the
	// stored write time are taken from.
	now func() time.Time
}

func New(opts Opts) *Service {
	return &Service{
		log:    opts.Logger.WithComponent("service"),
		repo:   opts.Repo,
		mapper: opts.Mapper,
		moddb:  opts.ModDB,
		now:    time.Now,
	}
}
