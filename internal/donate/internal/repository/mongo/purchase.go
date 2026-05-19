package repository

import (
	"context"

	dto "github.com/lasthearth/vsservice/internal/donate/internal/dto/mongo"
	"github.com/lasthearth/vsservice/internal/donate/internal/ierror"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	mgo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

func (r *Repository) createPurchase(ctx context.Context, purchase *model.Purchase) (*model.Purchase, error) {
	m := mongox.NewModel()
	d := dto.Purchase{
		Model:      m,
		PlayerID:   purchase.PlayerID,
		PlayerName: purchase.PlayerName,
		ItemID:     purchase.ItemID,
		ItemName:   purchase.ItemName,
		PricePaid:  purchase.PricePaid,
		Status:     string(purchase.Status),
		RefundedAt: purchase.RefundedAt,
	}

	result, err := r.purchColl.InsertOne(ctx, d)
	if err != nil {
		return nil, err
	}

	oid, err := mongox.ParseAnyObjectID(result.InsertedID)
	if err != nil {
		return nil, err
	}

	purchase.Id = oid.Hex()
	purchase.CreatedAt = m.CreatedAt
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

	updatedDTO := dto.Purchase{
		Model:      d.Model,
		PlayerID:   updated.PlayerID,
		PlayerName: updated.PlayerName,
		ItemID:     updated.ItemID,
		ItemName:   updated.ItemName,
		PricePaid:  updated.PricePaid,
		Status:     string(updated.Status),
		RefundedAt: updated.RefundedAt,
	}

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
	defer cursor.Close(ctx)

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
