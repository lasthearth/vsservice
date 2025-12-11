package event

import (
	"context"

	"github.com/lasthearth/vsservice/internal/player/internal/model"
	"go.uber.org/zap"
)

type PlayerRepository interface {
	GetByUserGameName(ctx context.Context, userGameName string) (*model.Player, error)
	UpdateById(ctx context.Context, id string, p model.PlayerUpdate) error
	UpdateByUserGameName(ctx context.Context, userGameName string, p model.PlayerUpdate) error
}

func (b *Bus) Subscribe() {
	l := b.log.WithMethod("subscribe")
	err := b.playerTryJoin.Respond(
		b.onPlayerTryJoin,
	)
	if err != nil {
		l.Error(
			"failed to subscribe to player try join queue",
			zap.Error(err),
		)
	}

	err = b.playerJoin.Subscribe(
		b.onPlayerJoin,
	)
	if err != nil {
		l.Error(
			"failed to subscribe to player join queue",
			zap.Error(err),
		)
	}

	err = b.playerLeave.Subscribe(
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
	b.playerTryJoin.Unsubscribe()
	b.playerJoin.Unsubscribe()
	b.playerLeave.Unsubscribe()
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

func (b *Bus) onPlayerJoin(ctx context.Context, data PlayerJoinEvent) error {
	l := b.log.WithMethod("on-player-join")
	isOnline := true
	if err := b.playerRepo.UpdateByUserGameName(
		ctx,
		data.Stats.Name,
		model.PlayerUpdate{
			IsOnline: &isOnline,
		},
	); err != nil {
		l.Error(
			"failed to update player online status",
			zap.Error(err),
		)
	}
	return nil
}

func (b *Bus) onPlayerLeave(ctx context.Context, data PlayerLeaveEvent) error {
	l := b.log.WithMethod("on-player-leave")
	isOnline := false
	if err := b.playerRepo.UpdateByUserGameName(
		ctx,
		data.Stats.Name,
		model.PlayerUpdate{
			IsOnline: &isOnline,
		},
	); err != nil {
		l.Error(
			"failed to update player online status",
			zap.Error(err),
		)
	}
	return nil
}
