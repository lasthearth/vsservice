package repository

import (
	"context"
	"errors"
	"time"

	dto "github.com/lasthearth/vsservice/internal/donate/internal/dto/mongo"
	"github.com/lasthearth/vsservice/internal/donate/internal/ierror"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/pkg/mongox/orderby"
	"github.com/lasthearth/vsservice/internal/pkg/mongox/pagination"
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
		if errors.Is(err, mgo.ErrNoDocuments) {
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

	setFields := bson.D{{Key: "updated_at", Value: now}}
	setOnInsertFields := bson.D{
		{Key: "_id", Value: mongox.NewModel().Id},
		{Key: "created_at", Value: now},
	}
	// An empty playerName means the caller has no display name to report
	// (e.g. cross-domain credits like referral rewards). Only overwrite an
	// existing wallet's player_name when a non-empty name is supplied, so
	// such calls don't blank out a name set by a previous call.
	if playerName != "" {
		setFields = append(setFields, bson.E{Key: "player_name", Value: playerName})
	} else {
		setOnInsertFields = append(setOnInsertFields, bson.E{Key: "player_name", Value: playerName})
	}

	update := bson.D{
		{Key: "$inc", Value: bson.D{{Key: "coins", Value: amount}}},
		{Key: "$set", Value: setFields},
		{Key: "$setOnInsert", Value: setOnInsertFields},
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

// ListWallets returns all wallets sorted by coins DESC, cursor-paginated.
func (r *Repository) ListWallets(ctx context.Context, pageToken string, limit int64) ([]*model.Wallet, string, error) {
	l := r.log.With(zap.String("method", "ListWallets"))

	if limit <= 0 {
		limit = 25
	}

	sort := orderby.BuildSortOptions(&orderby.Info{
		MongoField: "coins",
		Direction:  orderby.Desc,
	})

	opts := []pagination.OptionFn{
		pagination.WithLimit(limit),
		pagination.WithSort(sort),
	}
	if pageToken != "" {
		opts = append(opts, pagination.WithNext(pageToken))
	}

	resp, err := pagination.Find[dto.Wallet](ctx, r.walletColl, opts...)
	if err != nil {
		if errors.Is(err, pagination.ErrNoData) {
			return nil, "", nil
		}
		l.Error("failed to list wallets", zap.Error(err))
		return nil, "", err
	}

	result := make([]*model.Wallet, len(resp.Data))
	for i, d := range resp.Data {
		result[i] = walletFromDTO(d)
	}
	return result, resp.Next, nil
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
		if errors.Is(err, mgo.ErrNoDocuments) {
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
		"_id":         d.Id(),
	})
	if err != nil {
		l.Error("failed to replace wallet", zap.Error(err))
		return err
	}

	return nil
}
