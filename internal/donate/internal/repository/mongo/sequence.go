package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

// Do runs steps in order inside one MongoDB session, so a later step reads the
// writes of an earlier one (causal consistency).
//
// It is NOT atomic. The deployed server is a standalone, not a replica set, so
// transactions are unavailable and nothing here rolls back: if step N fails,
// steps 1..N-1 stay committed. This is the honest replacement for the
// mongo.WithSession call that used to sit inside BuyItem/Refund and read like a
// transaction without being one. Ordering and compensation belong to the use
// case that owns the sequence.
func (r *Repository) Do(ctx context.Context, steps ...func(context.Context) error) error {
	session, err := r.client.StartSession()
	if err != nil {
		r.log.Error("failed to start session", zap.Error(err))
		return err
	}
	defer session.EndSession(ctx)

	return mongo.WithSession(ctx, session, func(sc context.Context) error {
		for _, step := range steps {
			if err := step(sc); err != nil {
				return err
			}
		}
		return nil
	})
}
