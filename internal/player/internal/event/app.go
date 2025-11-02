package event

import (
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/messaging"
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
	log            logger.Logger
	playerJoinQ    messaging.Queue[PlayerJoinEvent, struct{}]
	playerLeaveQ   messaging.Queue[PlayerLeaveEvent, struct{}]
	playerTryJoinQ messaging.Queue[PlayerTryJoinReqEvent, PlayerTryJoinRespEvent]
	playerRepo     PlayerRepository
}

func NewEventManager(opts Opts) *Bus {
	ptjq := messaging.NewNatsQueue[PlayerTryJoinReqEvent, PlayerTryJoinRespEvent](
		opts.NC,
		playerTryJoinSubject,
	)

	pjq := messaging.NewNatsQueue[PlayerJoinEvent, struct{}](
		opts.NC,
		playerJoinSubject,
	)

	plq := messaging.NewNatsQueue[PlayerLeaveEvent, struct{}](
		opts.NC,
		playerLeaveSubject,
	)

	return &Bus{
		playerJoinQ:    pjq,
		playerLeaveQ:   plq,
		playerTryJoinQ: ptjq,
		playerRepo:     opts.PlayerRepo,
		log:            opts.Log.WithComponent("player-event-bus"),
	}
}
