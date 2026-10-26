package repository

import (
	"context"
	"errors"
	"fmt"
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

// UpdateKit sets the site-owned metadata with a narrow $set (upsert:false):
// only the title and/or category the caller named, plus updated_at. A matched
// count of 0 means the game never captured this kit — vsservice never
// inserts, so that is ierror.ErrNotFound. items/code/created_at are the
// game's and are untouched; a nil field pointer means "leave unchanged".
func (r *Repository) UpdateKit(ctx context.Context, code string, title, category *string) (*model.KitDef, error) {
	set := bson.M{"updated_at": time.Now()}
	if title != nil {
		set["title"] = *title
	}
	if category != nil {
		set["category"] = *category
	}

	res, err := r.kitsColl.UpdateOne(
		ctx,
		bson.M{"code": code},
		bson.M{"$set": set},
	)
	if err != nil {
		return nil, err
	}
	if res.MatchedCount == 0 {
		return nil, ierror.ErrNotFound
	}
	return r.GetKit(ctx, code)
}

// RemoveKitItem removes the item at index with one pipeline update. The
// `items.<index>` exists filter keeps the update from running on a kit whose
// array is shorter than the index (a race with a re-capture): it matches
// nothing and reads as NotFound, never removing the wrong position. A
// whole-document replace is not used — the attr snapshots of the other items
// must survive untouched.
func (r *Repository) RemoveKitItem(ctx context.Context, code string, index int) (*model.KitDef, error) {
	filter := bson.M{
		"code":                         code,
		fmt.Sprintf("items.%d", index): bson.M{"$exists": true},
	}
	update := bson.A{bson.M{
		"$set": bson.M{
			"items": bson.M{
				"$concatArrays": bson.A{
					bson.M{"$slice": bson.A{"$items", index}},
					bson.M{"$slice": bson.A{"$items", index + 1, bson.M{"$size": "$items"}}},
				},
			},
			"updated_at": time.Now(),
		},
	}}

	res, err := r.kitsColl.UpdateOne(ctx, filter, update)
	if err != nil {
		return nil, err
	}
	if res.MatchedCount == 0 {
		return nil, ierror.ErrNotFound
	}
	return r.GetKit(ctx, code)
}

func kitFromDTO(d dto.Kit) *model.KitDef {
	category := d.Category
	if category == "" {
		// Old documents predate the field; the default reads as "kit".
		category = model.KitCategoryKit
	}
	return model.ReconstituteKitDef(
		d.Code, d.Title, category, itemsFromDTO(d.Items), d.CreatedAt, d.UpdatedAt,
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
