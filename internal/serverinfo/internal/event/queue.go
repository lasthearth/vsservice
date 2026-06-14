package event

import (
	"context"

	"github.com/lasthearth/vsservice/internal/serverinfo/internal/model"
	"go.uber.org/zap"
)

func (b *Bus) Subscribe() {
	l := b.log.WithMethod("subscribe")
	err := b.worldTime.Subscribe(
		b.onWorldTime,
	)
	if err != nil {
		l.Error(
			"failed to subscribe to world time queue",
			zap.Error(err),
		)
	}

	err = b.totalOnline.Subscribe(
		b.onTotalOnline,
	)
	if err != nil {
		l.Error(
			"failed to subscribe to total online queue",
			zap.Error(err),
		)
	}
}

func (b *Bus) Unsubscribe() {
	if err := b.worldTime.Unsubscribe(); err != nil {
		b.log.Error("failed to unsubscribe worldTime", zap.Error(err))
	}
	if err := b.totalOnline.Unsubscribe(); err != nil {
		b.log.Error("failed to unsubscribe totalOnline", zap.Error(err))
	}
}

func (b *Bus) onWorldTime(ctx context.Context, event WorldTimeEvent) error {
	l := b.log.WithMethod("onWorldTime")
	l.Debug("received world time event", zap.String("time", event.Time))

	return b.repo.Update(
		ctx,
		func(
			ctx context.Context,
			update *model.ServerInfo,
		) (*model.ServerInfo, error) {
			update.SetWorldTime(event.Time)
			return update, nil
		},
	)
}

func (b *Bus) onTotalOnline(ctx context.Context, event TotalOnlineEvent) error {
	l := b.log.WithMethod("onTotalOnline")
	l.Debug("received total online event", zap.Int("total", event.Count))

	return b.repo.Update(
		ctx,
		func(
			ctx context.Context,
			update *model.ServerInfo,
		) (*model.ServerInfo, error) {
			update.SetTotalOnline(event.Count)
			return update, nil
		},
	)
}
