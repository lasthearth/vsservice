package repository

import (
	"context"
	"time"

	"github.com/go-faster/errors"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"

	notifierdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/notifier"
)

// transactionTimeout bounds one InTransaction call, retries of transient errors
// included.
const transactionTimeout = 15 * time.Second

// GetNotifier implements service.SettlementDbRepository.
func (r *Repository) GetNotifier(ctx context.Context, settlementID string) (model.NotifierPlacement, bool, error) {
	var dto notifierdto.Notifier
	err := r.notifierColl.FindOne(ctx, bson.M{"settlement_id": settlementID}).Decode(&dto)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return model.NotifierPlacement{}, false, nil
		}
		r.log.Error("failed to find settlement notifier", zap.Error(err), zap.String("settlement_id", settlementID))
		return model.NotifierPlacement{}, false, err
	}

	return r.mapper.ToNotifierPlacement(dto), true, nil
}

// ListNotifiers implements service.SettlementDbRepository: every placement that
// stands. The game server inserts a document when a block is placed and deletes
// it when the block is broken, so the collection is the whole answer and the
// read needs no filter.
//
// The sort is on settlement_id, which the unique index already carries, so one
// world answers every read in the same order.
func (r *Repository) ListNotifiers(ctx context.Context) ([]model.NotifierPlacement, error) {
	l := r.log.WithMethod("list_notifiers")

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	found, err := r.notifierColl.Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "settlement_id", Value: 1}}))
	if err != nil {
		l.Error("find error", zap.Error(err))
		return nil, err
	}
	defer func() {
		if err := found.Close(ctx); err != nil {
			l.Error("cursor close failed", zap.Error(err))
		}
	}()

	var dtos []notifierdto.Notifier
	if err := found.All(ctx, &dtos); err != nil {
		l.Error("cursor error", zap.Error(err))
		return nil, err
	}

	placements := r.mapper.ToNotifierPlacements(dtos)
	l.Debug("notifier placements retrieved", zap.Int("count", len(placements)))
	return placements, nil
}

// InTransaction implements service.SettlementDbRepository.
func (r *Repository) InTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	session, err := r.client.StartSession()
	if err != nil {
		r.log.Error("failed to start session", zap.Error(err))
		return err
	}
	defer session.EndSession(ctx)

	ctx, cancel := context.WithTimeout(ctx, transactionTimeout)
	defer cancel()

	_, err = session.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		return nil, fn(ctx)
	})
	return err
}
