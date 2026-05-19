package repository

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// AddCoinsToWallet atomically upserts the donate_wallets collection.
// Mirrors the logic in internal/donate/internal/repository/mongo/wallet.go.
func (r *Repository) AddCoinsToWallet(ctx context.Context, playerID, playerName string, amount int64) error {
	now := time.Now()
	filter := bson.M{"player_id": playerID}
	update := bson.D{
		{Key: "$inc", Value: bson.D{{Key: "coins", Value: amount}}},
		{Key: "$set", Value: bson.D{
			{Key: "updated_at", Value: now},
			{Key: "player_name", Value: playerName},
		}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "_id", Value: mongox.NewModel().Id},
			{Key: "created_at", Value: now},
		}},
	}

	_, err := r.walletColl.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true))
	if err != nil {
		r.log.Error("AddCoinsToWallet: upsert failed", zap.String("player_id", playerID), zap.Error(err))
	}
	return err
}

// CreateCreditTransaction inserts a credit record into the donate_transactions collection.
func (r *Repository) CreateCreditTransaction(ctx context.Context, playerID string, amount int64, reason string) error {
	m := mongox.NewModel()
	doc := bson.D{
		{Key: "_id", Value: m.Id},
		{Key: "player_id", Value: playerID},
		{Key: "amount", Value: amount},
		{Key: "type", Value: "credit"},
		{Key: "reason", Value: reason},
		{Key: "created_at", Value: m.CreatedAt},
		{Key: "updated_at", Value: m.UpdatedAt},
	}

	if _, err := r.txColl.InsertOne(ctx, doc); err != nil {
		r.log.Error("CreateCreditTransaction: insert failed", zap.String("player_id", playerID), zap.Error(err))
		return err
	}
	return nil
}
