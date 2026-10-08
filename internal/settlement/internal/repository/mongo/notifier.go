package repository

import (
	"context"
	"time"

	"github.com/go-faster/errors"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
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
