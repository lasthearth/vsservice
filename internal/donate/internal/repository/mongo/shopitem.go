package repository

import (
	"context"
	"time"

	dto "github.com/lasthearth/vsservice/internal/donate/internal/dto/mongo"
	"github.com/lasthearth/vsservice/internal/donate/internal/ierror"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	mgo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

func (r *Repository) CreateShopItem(ctx context.Context, item *model.ShopItem) (*model.ShopItem, error) {
	l := r.log.With(zap.String("method", "CreateShopItem"))

	m := mongox.NewModel()
	now := m.CreatedAt
	d := shopItemToDTO(item)
	d.Model = m

	result, err := r.shopColl.InsertOne(ctx, d)
	if err != nil {
		l.Error("failed to insert shop item", zap.Error(err))
		return nil, err
	}

	oid, err := mongox.ParseAnyObjectID(result.InsertedID)
	if err != nil {
		return nil, err
	}

	item.MarkCreated(oid.Hex(), now)
	return item, nil
}

func (r *Repository) GetShopItem(ctx context.Context, id string) (*model.ShopItem, error) {
	l := r.log.With(zap.String("method", "GetShopItem"), zap.String("id", id))

	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, ierror.ErrNotFound
	}

	var d dto.ShopItem
	if err := r.shopColl.FindOne(ctx, bson.M{"_id": oid}).Decode(&d); err != nil {
		if err == mgo.ErrNoDocuments {
			return nil, ierror.ErrNotFound
		}
		l.Error("failed to find shop item", zap.Error(err))
		return nil, err
	}

	return shopItemFromDTO(d), nil
}

func (r *Repository) UpdateShopItem(
	ctx context.Context,
	id string,
	updateFn func(ctx context.Context, item *model.ShopItem) (*model.ShopItem, error),
) (*model.ShopItem, error) {
	l := r.log.With(zap.String("method", "UpdateShopItem"), zap.String("id", id))

	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, ierror.ErrNotFound
	}

	var d dto.ShopItem
	if err := r.shopColl.FindOne(ctx, bson.M{"_id": oid}).Decode(&d); err != nil {
		if err == mgo.ErrNoDocuments {
			return nil, ierror.ErrNotFound
		}
		l.Error("failed to find shop item", zap.Error(err))
		return nil, err
	}

	item := shopItemFromDTO(d)
	updated, err := updateFn(ctx, item)
	if err != nil {
		return nil, err
	}

	updated.Touch(time.Now())
	updatedDTO := shopItemToDTO(updated)

	if _, err := r.shopColl.ReplaceOne(ctx, bson.M{"_id": oid}, updatedDTO); err != nil {
		l.Error("failed to replace shop item", zap.Error(err))
		return nil, err
	}

	return updated, nil
}

func (r *Repository) DeleteShopItem(ctx context.Context, id string) error {
	l := r.log.With(zap.String("method", "DeleteShopItem"), zap.String("id", id))

	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return ierror.ErrNotFound
	}

	result, err := r.shopColl.DeleteOne(ctx, bson.M{"_id": oid})
	if err != nil {
		l.Error("failed to delete shop item", zap.Error(err))
		return err
	}
	if result.DeletedCount == 0 {
		return ierror.ErrNotFound
	}
	return nil
}

func (r *Repository) ListShopItems(ctx context.Context, availableOnly bool) ([]*model.ShopItem, error) {
	l := r.log.With(zap.String("method", "ListShopItems"))

	filter := bson.M{}
	if availableOnly {
		filter["is_available"] = true
	}

	cursor, err := r.shopColl.Find(ctx, filter)
	if err != nil {
		l.Error("failed to find shop items", zap.Error(err))
		return nil, err
	}
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			l.Error("cursor close failed", zap.Error(err))
		}
	}()

	var dtos []dto.ShopItem
	if err := cursor.All(ctx, &dtos); err != nil {
		l.Error("failed to decode shop items", zap.Error(err))
		return nil, err
	}

	result := make([]*model.ShopItem, len(dtos))
	for i, d := range dtos {
		result[i] = shopItemFromDTO(d)
	}
	return result, nil
}
