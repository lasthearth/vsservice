package repository

import (
	"context"
	"errors"
	"time"

	dto "github.com/lasthearth/vsservice/internal/kitdef/internal/dto/mongo"
	"github.com/lasthearth/vsservice/internal/kitdef/internal/ierror"
	"github.com/lasthearth/vsservice/internal/kitdef/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	mgo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

func (r *Repository) ListKits(ctx context.Context) ([]*model.KitDef, error) {
	l := r.log.With(zap.String("method", "ListKits"))

	cursor, err := r.kitsColl.Find(ctx, bson.M{})
	if err != nil {
		l.Error("failed to find kits", zap.Error(err))
		return nil, err
	}
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			l.Error("cursor close failed", zap.Error(err))
		}
	}()

	var dtos []dto.Kit
	if err := cursor.All(ctx, &dtos); err != nil {
		l.Error("failed to decode kits", zap.Error(err))
		return nil, err
	}

	result := make([]*model.KitDef, len(dtos))
	for i, d := range dtos {
		result[i] = kitFromDTO(d)
	}
	return result, nil
}

func (r *Repository) GetKit(ctx context.Context, code string) (*model.KitDef, error) {
	var d dto.Kit
	if err := r.kitsColl.FindOne(ctx, bson.M{"code": code}).Decode(&d); err != nil {
		if errors.Is(err, mgo.ErrNoDocuments) {
			return nil, ierror.ErrNotFound
		}
		return nil, err
	}
	return kitFromDTO(d), nil
}

// RenameKit sets title metadata only, with upsert:false. A matched count of 0
// means the game never captured this kit — vsservice never inserts, so that is
// ierror.ErrNotFound. items/code/created_at are the game's and are untouched.
func (r *Repository) RenameKit(ctx context.Context, code, title string) (*model.KitDef, error) {
	res, err := r.kitsColl.UpdateOne(
		ctx,
		bson.M{"code": code},
		bson.M{"$set": bson.M{"title": title, "updated_at": time.Now()}},
	)
	if err != nil {
		return nil, err
	}
	if res.MatchedCount == 0 {
		return nil, ierror.ErrNotFound
	}
	return r.GetKit(ctx, code)
}

func kitFromDTO(d dto.Kit) *model.KitDef {
	return model.ReconstituteKitDef(
		d.Code, d.Title, itemsFromDTO(d.Items), d.CreatedAt, d.UpdatedAt,
	)
}

func itemsFromDTO(items []dto.KitItem) []model.KitItem {
	if items == nil {
		return nil
	}
	out := make([]model.KitItem, len(items))
	for i, it := range items {
		out[i] = model.KitItem{
			GameCode:     it.GameCode,
			Type:         it.Type,
			Quantity:     it.Quantity,
			AttrSnapshot: it.AttrSnapshot,
			ImageURL:     it.ImageURL,
		}
	}
	return out
}
