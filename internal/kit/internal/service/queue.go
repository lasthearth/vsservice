package service

import (
	"context"

	"github.com/lasthearth/vsservice/internal/kit/internal/model"
	"go.uber.org/zap"
)

func (b *Bus) Subscribe() {
	l := b.log.WithMethod("subscribe")

	err := b.kitClaimedSub.Subscribe(
		b.onKitReceived,
	)
	if err != nil {
		l.Error(
			"failed to subscribe to kit received event queue",
			zap.Error(err),
		)
	}
}

func (b *Bus) Unsubscribe() {
	b.kitClaimedSub.Unsubscribe()
}

func (b *Bus) onKitReceived(ctx context.Context, data KitClaimedEvent) error {
	err := b.assignmentRepo.UpdateAssignment(
		ctx,
		data.AssignmentID,
		func(
			ctx context.Context,
			assignment *model.KitAssignment,
		) (*model.KitAssignment, error) {
			if err := assignment.TransitionTo(model.AssignmentStatusDelivered); err != nil {
				return nil, err
			}
			return assignment, nil
		},
	)
	if err != nil {
		b.log.Error(
			"failed to update assignment status",
			zap.Error(err),
		)
		return err
	}

	return nil
}
