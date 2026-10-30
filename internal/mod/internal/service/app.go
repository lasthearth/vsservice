package service

import (
	"time"

	modv1 "github.com/lasthearth/vsservice/gen/mod/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
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
	// Config is optional: without it the archive has no size cap.
	Config config.Config `optional:"true"`
	// Syncer is optional: a unit test that does not exercise the manifest sync
	// builds the service without one, and ListMods then skips the refresh.
	Syncer *Syncer `optional:"true"`
}

type Service struct {
	log    logger.Logger
	repo   Repository
	mapper Mapper
	moddb  ModDB
	// syncer refreshes the server-sourced mods when the cached list is stale.
	syncer *Syncer
	// archiveMax is the archive's size cap; 0 — none.
	archiveMax int64
	// now is a field so a test can pin the clock the archive timestamp and the
	// stored write time are taken from.
	now func() time.Time
}

func New(opts Opts) *Service {
	return &Service{
		log:        opts.Logger.WithComponent("service"),
		repo:       opts.Repo,
		mapper:     opts.Mapper,
		moddb:      opts.ModDB,
		syncer:     opts.Syncer,
		archiveMax: opts.Config.ModsArchiveMaxBytes,
		now:        time.Now,
	}
}
