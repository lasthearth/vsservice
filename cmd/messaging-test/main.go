package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/nats-io/nats.go"
)

type PlayerTryJoinReqEvent struct {
	PlayerName string `json:"player_name"`
	PlayerUid  string `json:"player_uid"`
}

type PlayerTryJoinResEvent struct {
	Status string `json:"status"`
}

func main() {
	c, err := config.New()
	if err != nil {
		panic(err)
	}

	fmt.Println(c.NatsUrl)
	nc, err := nats.Connect(c.NatsUrl)
	if err != nil {
		panic(err)
	}
	queue := messaging.NewNatsQueue[PlayerTryJoinReqEvent, PlayerTryJoinResEvent](
		nc,
		"player.try-join",
		time.Second*2,
		messaging.JsonEncoder,
	)

	queue.Subscribe(func(
		ctx context.Context,
		data PlayerTryJoinReqEvent,
	) (PlayerTryJoinResEvent, error) {
		fmt.Println(data.PlayerName)
		fmt.Println(data.PlayerUid)

		return PlayerTryJoinResEvent{
			Status: "verified",
		}, nil
	})
	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait()
}
