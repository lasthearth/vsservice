package event

import (
	"context"

	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/lasthearth/vsservice/internal/player/internal/model"
	"go.uber.org/zap"
)

type PlayerRepository interface {
	GetByUserGameName(ctx context.Context, userGameName string) (*model.Player, error)
	UpdateById(ctx context.Context, id string, p model.Player) error
	UpdateByUserGameName(ctx context.Context, userGameName string, p model.PlayerUpdate) error
}

func (b *Bus) Subscribe() {
	l := b.log.WithMethod("subscribe")
	err := b.playerTryJoinQ.SubscribeGroup(
		messaging.DefaultQueueGroup,
		b.onPlayerTryJoin,
	)
	if err != nil {
		l.Error(
			"failed to subscribe to player try join queue",
			zap.Error(err),
		)
	}

	err = b.playerJoinQ.SubscribeGroup(
		messaging.DefaultQueueGroup,
		b.onPlayerJoin,
	)
	if err != nil {
		l.Error(
			"failed to subscribe to player join queue",
			zap.Error(err),
		)
	}

	err = b.playerLeaveQ.SubscribeGroup(
		messaging.DefaultQueueGroup,
		b.onPlayerLeave,
	)
	if err != nil {
		l.Error(
			"failed to subscribe to player leave queue",
			zap.Error(err),
		)
	}
}

func (b *Bus) Unsubscribe() {
	b.playerTryJoinQ.Unsubscribe()
	b.playerJoinQ.Unsubscribe()
	b.playerLeaveQ.Unsubscribe()
}

func (b *Bus) onPlayerTryJoin(ctx context.Context, data PlayerTryJoinReqEvent) (PlayerTryJoinRespEvent, error) {
	player, err := b.playerRepo.GetByUserGameName(ctx, data.PlayerName)
	if err != nil {
		return PlayerTryJoinRespEvent{
			Status: "not_found",
		}, err
	}

	return PlayerTryJoinRespEvent{
		Status: string(player.Verification.Status),
	}, nil
}

func (b *Bus) onPlayerJoin(ctx context.Context, data PlayerJoinEvent) (struct{}, error) {
	isOnline := true
	if err := b.playerRepo.UpdateByUserGameName(
		ctx,
		data.Stats.Name,
		model.PlayerUpdate{
			IsOnline: &isOnline,
		},
	); err != nil {
		return struct{}{}, err
	}

	return struct{}{}, nil
}

func (b *Bus) onPlayerLeave(ctx context.Context, data PlayerLeaveEvent) (struct{}, error) {
	isOnline := false
	if err := b.playerRepo.UpdateByUserGameName(
		ctx,
		data.Stats.Name,
		model.PlayerUpdate{
			IsOnline: &isOnline,
		},
	); err != nil {
		return struct{}{}, err
	}

	return struct{}{}, nil
}
