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
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

func (r *Repository) GetWalletByPlayerID(ctx context.Context, playerID string) (*model.Wallet, error) {
	l := r.log.With(zap.String("method", "GetWalletByPlayerID"), zap.String("player_id", playerID))

	var d dto.Wallet
	err := r.walletColl.FindOne(ctx, bson.M{"player_id": playerID}).Decode(&d)
	if err != nil {
		if err == mgo.ErrNoDocuments {
			return nil, ierror.ErrNotFound
		}
		l.Error("failed to find wallet", zap.Error(err))
		return nil, err
	}

	return walletFromDTO(d), nil
}

// AddCoinsToWallet atomically upserts the wallet and increments coins by amount.
func (r *Repository) AddCoinsToWallet(ctx context.Context, playerID, playerName string, amount int64) (int64, error) {
	l := r.log.With(zap.String("method", "AddCoinsToWallet"), zap.String("player_id", playerID))

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
	opts := options.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(options.After)

	var d dto.Wallet
	err := r.walletColl.FindOneAndUpdate(ctx, filter, update, opts).Decode(&d)
	if err != nil {
		l.Error("failed to add coins", zap.Error(err))
		return 0, err
	}

	return d.Coins, nil
}

// UpdateWallet reads the wallet, applies updateFn, then replaces the document.
func (r *Repository) UpdateWallet(
	ctx context.Context,
	playerID string,
	updateFn func(ctx context.Context, wallet *model.Wallet) (*model.Wallet, error),
) error {
	l := r.log.With(zap.String("method", "UpdateWallet"), zap.String("player_id", playerID))

	var d dto.Wallet
	err := r.walletColl.FindOne(ctx, bson.M{"player_id": playerID}).Decode(&d)
	if err != nil {
		if err == mgo.ErrNoDocuments {
			return ierror.ErrNotFound
		}
		l.Error("failed to find wallet", zap.Error(err))
		return err
	}

	wallet := walletFromDTO(d)
	updated, err := updateFn(ctx, wallet)
	if err != nil {
		return err
	}

	updated.Touch(time.Now())
	_, err = r.walletColl.ReplaceOne(ctx, bson.M{"player_id": playerID}, bson.M{
		"player_id":   updated.PlayerID,
		"player_name": updated.PlayerName,
		"coins":       updated.Coins,
		"created_at":  d.CreatedAt,
		"updated_at":  updated.UpdatedAt,
		"_id":         d.Id,
	})
	if err != nil {
		l.Error("failed to replace wallet", zap.Error(err))
		return err
	}

	return nil
}
