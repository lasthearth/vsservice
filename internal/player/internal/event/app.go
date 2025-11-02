package event

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/nats-io/nats.go"
	"go.uber.org/fx"
)

const (
	module       = "player-event"
	tryJoinTopic = "player.try-join"
)

type Opts struct {
	fx.In
	NC               *nats.Conn
	PlayerRepo       PlayerRepository
	PlayerRepository PlayerRepository
}

type Bus struct {
	playerTryJoinQ messaging.Queue[PlayerTryJoinReqEvent, PlayerTryJoinRespEvent]
	playerRepo     PlayerRepository
}

func NewEventManager(opts Opts) *Bus {
	ptjq := messaging.NewNatsQueue[PlayerTryJoinReqEvent, PlayerTryJoinRespEvent](
		opts.NC,
		tryJoinTopic,
		time.Second*2,
		messaging.JsonEncoder,
	)
	return &Bus{
		playerTryJoinQ: ptjq,
		playerRepo:     opts.PlayerRepo,
	}
}
