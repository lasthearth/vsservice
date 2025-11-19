package event

import (
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/lasthearth/vsservice/internal/pkg/messaging/mnats"
	"github.com/nats-io/nats.go"
	"go.uber.org/fx"
)

const (
	module               = "player-event"
	playerTryJoinSubject = "player.try-join"
	playerJoinSubject    = "player.join"
	playerLeaveSubject   = "player.leave"
)

type Opts struct {
	fx.In
	NC               *nats.Conn
	Log              logger.Logger
	PlayerRepo       PlayerRepository
	PlayerRepository PlayerRepository
}

type Bus struct {
	log           logger.Logger
	playerJoin    messaging.Subscriber[PlayerJoinEvent]
	playerLeave   messaging.Subscriber[PlayerLeaveEvent]
	playerTryJoin messaging.RpcResponder[PlayerTryJoinReqEvent, PlayerTryJoinRespEvent]
	playerRepo    PlayerRepository
}

func NewEventManager(opts Opts) *Bus {
	presp := mnats.NewRpcResponder[PlayerTryJoinReqEvent, PlayerTryJoinRespEvent](
		opts.NC,
		playerTryJoinSubject,
		messaging.DefaultWorkerGroup,
	)

	pjs := mnats.NewEventSubscriber[PlayerJoinEvent](
		opts.NC,
		playerJoinSubject,
		messaging.DefaultWorkerGroup,
	)

	pls := mnats.NewEventSubscriber[PlayerLeaveEvent](
		opts.NC,
		playerLeaveSubject,
		messaging.DefaultWorkerGroup,
	)

	return &Bus{
		playerJoin:    pjs,
		playerLeave:   pls,
		playerTryJoin: presp,
		playerRepo:    opts.PlayerRepo,
		log:           opts.Log.WithComponent("player-event-bus"),
	}
}
