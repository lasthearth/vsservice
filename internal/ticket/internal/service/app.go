package service

import (
	"time"

	ticketv1 "github.com/lasthearth/vsservice/gen/ticket/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

var _ ticketv1.TicketServiceServer = (*Service)(nil)

type Opts struct {
	fx.In
	Logger   logger.Logger
	Repo     Repository
	Notifier Notifier
}

type Service struct {
	logger   logger.Logger
	repo     Repository
	notifier Notifier
	now      func() time.Time
}

func New(opts Opts) *Service {
	return &Service{
		logger:   opts.Logger.WithComponent("service"),
		repo:     opts.Repo,
		notifier: opts.Notifier,
		now:      func() time.Time { return time.Now().UTC().Truncate(time.Millisecond) },
	}
}
