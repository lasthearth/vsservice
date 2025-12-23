package event

import (
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/lasthearth/vsservice/internal/pkg/messaging/mnats"
	"github.com/lasthearth/vsservice/internal/serverinfo/internal/service/serverinfo"
	"github.com/nats-io/nats.go"
	"go.uber.org/fx"
)

const (
	module             = "serverinfo-event"
	worldTimeSubject   = "world.time"
	totalOnlineSubject = "world.total-online"
)

type Opts struct {
	fx.In
	NC   *nats.Conn
	Log  logger.Logger
	Repo serverinfo.ServerInfoRepository
}

type Bus struct {
	log         logger.Logger
	repo        serverinfo.ServerInfoRepository
	worldTime   messaging.Subscriber[WorldTimeEvent]
	totalOnline messaging.Subscriber[TotalOnlineEvent]
}

func NewEventManagerFx(opts Opts) *Bus {
	return NewEventManager(opts.NC, opts.Log, opts.Repo)
}

func NewEventManager(
	nc *nats.Conn,
	log logger.Logger,
	repo serverinfo.ServerInfoRepository,
) *Bus {
	worldTime := mnats.NewEventSubscriber[WorldTimeEvent](
		nc,
		worldTimeSubject,
		messaging.DefaultWorkerGroup,
		mnats.WithLogger(log),
	)

	totalOnline := mnats.NewEventSubscriber[TotalOnlineEvent](
		nc,
		totalOnlineSubject,
		messaging.DefaultWorkerGroup,
		mnats.WithLogger(log),
	)

	return &Bus{
		log:         log.WithComponent("serverinfo-event-bus"),
		repo:        repo,
		worldTime:   worldTime,
		totalOnline: totalOnline,
	}
}
