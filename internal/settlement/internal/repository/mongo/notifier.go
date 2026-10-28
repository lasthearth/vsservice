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

// GetNotifier implements service.SettlementDbRepository. It reads the bound
// collection only: a neutral herald belongs to no settlement, so it never locks
// the coordinates of an upgrade request.
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

// ListNotifiers implements service.SettlementDbRepository: every herald that
// stands, bound or not. The game server inserts a row when a block is placed
// and deletes it when the block is broken, so the two collections are the whole
// answer and the read needs no filter.
//
// Two collections, because the two kinds of herald obey different keys: a bound
// one is unique per settlement, a neutral one unique per position. A block is
// in exactly one of them — binding a neutral herald retires its row — so the
// answer is the concatenation and never holds one block twice.
//
// Bound rows come first, each sorted by the settlement_id its unique index
// carries, then the neutral ones by position, so one world answers every read
// in the same order.
func (r *Repository) ListNotifiers(ctx context.Context) ([]model.NotifierPlacement, error) {
	l := r.log.WithMethod("list_notifiers")

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	bound, err := r.listBoundNotifiers(ctx)
	if err != nil {
		return nil, err
	}
	unbound, err := r.listUnboundNotifiers(ctx)
	if err != nil {
		return nil, err
	}

	l.Debug("notifier placements retrieved",
		zap.Int("bound", len(bound)), zap.Int("unbound", len(unbound)))
	return append(bound, withoutStandingTwice(bound, unbound)...), nil
}

// withoutStandingTwice drops the neutral rows whose position a bound herald
// already occupies, so one block is one pin.
//
// Binding a neutral herald retires its row, but the two writes are not atomic,
// and a retirement that never landed leaves the block in both collections for
// good. The bound row wins: it carries the settlement the map names the pin
// after, and the neutral one is only its leftover.
func withoutStandingTwice(bound, unbound []model.NotifierPlacement) []model.NotifierPlacement {
	if len(unbound) == 0 {
		return unbound
	}

	taken := make(map[model.Vector3]struct{}, len(bound))
	for _, placement := range bound {
		taken[placement.Position] = struct{}{}
	}

	out := make([]model.NotifierPlacement, 0, len(unbound))
	for _, placement := range unbound {
		if _, standing := taken[placement.Position]; standing {
			continue
		}
		out = append(out, placement)
	}
	return out
}

// listBoundNotifiers reads settlement_notifiers, one row per settlement.
func (r *Repository) listBoundNotifiers(ctx context.Context) ([]model.NotifierPlacement, error) {
	l := r.log.WithMethod("list_notifiers")

	found, err := r.notifierColl.Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "settlement_id", Value: 1}}))
	if err != nil {
		l.Error("find error", zap.Error(err), zap.String("collection", r.notifierColl.Name()))
		return nil, err
	}
	defer func() {
		if err := found.Close(ctx); err != nil {
			l.Error("cursor close failed", zap.Error(err))
		}
	}()

	var dtos []notifierdto.Notifier
	if err := found.All(ctx, &dtos); err != nil {
		l.Error("cursor error", zap.Error(err), zap.String("collection", r.notifierColl.Name()))
		return nil, err
	}
	return r.mapper.ToNotifierPlacements(dtos), nil
}

// listUnboundNotifiers reads unbound_notifiers, one row per block position. A
// neutral herald has no settlement id, so it is served with an empty one: that
// is what tells the map the pin is neutral.
func (r *Repository) listUnboundNotifiers(ctx context.Context) ([]model.NotifierPlacement, error) {
	l := r.log.WithMethod("list_notifiers")

	found, err := r.unboundNotifierColl.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{
		{Key: "position.x", Value: 1},
		{Key: "position.y", Value: 1},
		{Key: "position.z", Value: 1},
	}))
	if err != nil {
		l.Error("find error", zap.Error(err), zap.String("collection", r.unboundNotifierColl.Name()))
		return nil, err
	}
	defer func() {
		if err := found.Close(ctx); err != nil {
			l.Error("cursor close failed", zap.Error(err))
		}
	}()

	var dtos []notifierdto.Unbound
	if err := found.All(ctx, &dtos); err != nil {
		l.Error("cursor error", zap.Error(err), zap.String("collection", r.unboundNotifierColl.Name()))
		return nil, err
	}
	return r.mapper.ToUnboundPlacements(dtos), nil
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
