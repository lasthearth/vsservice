package repository

import (
	"context"
	"errors"

	dto "github.com/lasthearth/vsservice/internal/donate/internal/dto/mongo"
	"github.com/lasthearth/vsservice/internal/donate/internal/ierror"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/pkg/mongox/pagination"
	"go.mongodb.org/mongo-driver/v2/bson"
	mgo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

func (r *Repository) createPurchase(ctx context.Context, purchase *model.Purchase) (*model.Purchase, error) {
	m := mongox.NewModel()
	d := purchaseToDTO(m, purchase)

	result, err := r.purchColl.InsertOne(ctx, d)
	if err != nil {
		return nil, err
	}

	oid, err := mongox.ParseAnyObjectID(result.InsertedID)
	if err != nil {
		return nil, err
	}

	purchase.MarkCreated(oid.Hex(), m.CreatedAt)
	return purchase, nil
}

func (r *Repository) GetPurchase(ctx context.Context, id string) (*model.Purchase, error) {
	l := r.log.With(zap.String("method", "GetPurchase"), zap.String("id", id))

	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, ierror.ErrNotFound
	}

	var d dto.Purchase
	if err := r.purchColl.FindOne(ctx, bson.M{"_id": oid}).Decode(&d); err != nil {
		if err == mgo.ErrNoDocuments {
			return nil, ierror.ErrNotFound
		}
		l.Error("failed to find purchase", zap.Error(err))
		return nil, err
	}

	return purchaseFromDTO(d), nil
}

func (r *Repository) updatePurchase(
	ctx context.Context,
	id string,
	updateFn func(ctx context.Context, p *model.Purchase) (*model.Purchase, error),
) (*model.Purchase, error) {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, ierror.ErrNotFound
	}

	var d dto.Purchase
	if err := r.purchColl.FindOne(ctx, bson.M{"_id": oid}).Decode(&d); err != nil {
		if err == mgo.ErrNoDocuments {
			return nil, ierror.ErrNotFound
		}
		return nil, err
	}

	purchase := purchaseFromDTO(d)
	updated, err := updateFn(ctx, purchase)
	if err != nil {
		return nil, err
	}

	updatedDTO := purchaseToDTO(d.Model, updated)

	if _, err := r.purchColl.ReplaceOne(ctx, bson.M{"_id": oid}, updatedDTO); err != nil {
		return nil, err
	}

	return updated, nil
}

func (r *Repository) ListPurchasesByPlayerID(ctx context.Context, playerID string) ([]*model.Purchase, error) {
	l := r.log.With(zap.String("method", "ListPurchasesByPlayerID"), zap.String("player_id", playerID))

	cursor, err := r.purchColl.Find(ctx, bson.M{"player_id": playerID})
	if err != nil {
		l.Error("failed to find purchases", zap.Error(err))
		return nil, err
	}
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			l.Error("cursor close failed", zap.Error(err))
		}
	}()

	var dtos []dto.Purchase
	if err := cursor.All(ctx, &dtos); err != nil {
		l.Error("failed to decode purchases", zap.Error(err))
		return nil, err
	}

	result := make([]*model.Purchase, len(dtos))
	for i, d := range dtos {
		result[i] = purchaseFromDTO(d)
	}
	return result, nil
}

// MarkPurchaseIssued marks a purchase as manually delivered by adminID.
// Idempotent: re-issuing a purchase that's already issued is a no-op.
// Returns ierror.ErrCannotIssueRefunded if the purchase is refunded.
func (r *Repository) MarkPurchaseIssued(ctx context.Context, purchaseID, adminID string) (*model.Purchase, error) {
	return r.updatePurchase(ctx, purchaseID, func(_ context.Context, p *model.Purchase) (*model.Purchase, error) {
		if err := p.MarkIssued(adminID); err != nil {
			return nil, ierror.ErrCannotIssueRefunded
		}
		return p, nil
	})
}

// ListPendingPurchases returns active purchases that have not yet been marked as issued.
// Cursor-paginated; pass an empty pageToken for the first page.
func (r *Repository) ListPendingPurchases(ctx context.Context, pageToken string, limit int64) ([]*model.Purchase, string, error) {
	l := r.log.With(zap.String("method", "ListPendingPurchases"))

	if limit <= 0 {
		limit = 25
	}

	opts := []pagination.OptionFn{
		pagination.WithLimit(limit),
		pagination.WithFilter(bson.M{
			"issued_at": bson.M{"$exists": false},
			"status":    string(model.PurchaseStatusActive),
		}),
	}
	if pageToken != "" {
		opts = append(opts, pagination.WithNext(pageToken))
	}

	resp, err := pagination.Find[dto.Purchase](ctx, r.purchColl, opts...)
	if err != nil {
		if errors.Is(err, pagination.ErrNoData) || errors.Is(err, mgo.ErrNoDocuments) {
			return nil, "", nil
		}
		l.Error("failed to list pending purchases", zap.Error(err))
		return nil, "", err
	}

	result := make([]*model.Purchase, len(resp.Data))
	for i, d := range resp.Data {
		result[i] = purchaseFromDTO(d)
	}
	return result, resp.Next, nil
}
