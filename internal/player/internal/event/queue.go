package event

import (
	"context"
	"fmt"

	"github.com/lasthearth/vsservice/internal/player/internal/model"
)

type PlayerRepository interface {
	GetByUserGameName(ctx context.Context, userGameName string) (*model.Player, error)
}

func (b *Bus) Subscribe() {
	ptjqWG := fmt.Sprintf("%s.group", tryJoinTopic)
	b.playerTryJoinQ.SubscribeGroup(
		ptjqWG,
		b.onPlayerTryJoin,
	)
}

func (b *Bus) Unsubscribe() {
	b.playerTryJoinQ.Unsubscribe()
}

func (b *Bus) onPlayerTryJoin(ctx context.Context, data PlayerTryJoinReqEvent) (PlayerTryJoinRespEvent, error) {
	player, err := b.playerRepo.GetByUserGameName(ctx, data.PlayerName)
	if err != nil {
		return PlayerTryJoinRespEvent{
			Status: "not_found",
		}, nil
	}

	return PlayerTryJoinRespEvent{
		Status: string(player.Verification.Status),
	}, nil
}

func (b *Bus) onPlayerJoin(ctx context.Context, data PlayerJoinEvent) error {
	return nil
}
