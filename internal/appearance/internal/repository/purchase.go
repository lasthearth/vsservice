package repository

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/appearance/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

// purchase is a document of the appearance_purchases collection.
type purchase struct {
	UserId    string    `bson:"user_id"`
	ItemId    string    `bson:"item_id"`
	Price     int64     `bson:"price"`
	CreatedAt time.Time `bson:"created_at"`
}

// AddPurchase records that the player bought the item. Buying it again is
// model.ErrAlreadyOwned.
func (r *Repository) AddPurchase(ctx context.Context, userID, itemID string, price int64, at time.Time) error {
	_, err := r.purchases.InsertOne(ctx, purchase{UserId: userID, ItemId: itemID, Price: price, CreatedAt: at})
	if mongo.IsDuplicateKeyError(err) {
		return model.ErrAlreadyOwned
	}
	if err != nil {
		r.logger.WithMethod("AddPurchase").Error("failed to record purchase", zap.Error(err))
	}
	return err
}

// purchased returns the ids of the items the player bought.
func (s *Standings) purchased(ctx context.Context, userID string) (map[string]bool, error) {
	cursor, err := s.purchases.Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, err
	}
	var docs []purchase
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	owned := make(map[string]bool, len(docs))
	for _, d := range docs {
		owned[d.ItemId] = true
	}
	return owned, nil
}
