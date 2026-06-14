package repository

import (
	"context"

	dto "github.com/lasthearth/vsservice/internal/donate/internal/dto/mongo"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"
)

func (r *Repository) CreateTransaction(ctx context.Context, tx *model.Transaction) (*model.Transaction, error) {
	l := r.log.With(zap.String("method", "CreateTransaction"), zap.String("player_id", tx.PlayerID))

	m := mongox.NewModel()
	d := dto.Transaction{
		Model:      m,
		PlayerID:   tx.PlayerID,
		Amount:     tx.Amount,
		Type:       string(tx.Type),
		Reason:     tx.Reason,
		PurchaseID: tx.PurchaseID,
	}

	result, err := r.txColl.InsertOne(ctx, d)
	if err != nil {
		l.Error("failed to insert transaction", zap.Error(err))
		return nil, err
	}

	oid, err := mongox.ParseAnyObjectID(result.InsertedID)
	if err != nil {
		return nil, err
	}

	tx.MarkCreated(oid.Hex(), m.CreatedAt)
	return tx, nil
}

func (r *Repository) ListTransactionsByPlayerID(ctx context.Context, playerID string) ([]*model.Transaction, error) {
	l := r.log.With(zap.String("method", "ListTransactionsByPlayerID"), zap.String("player_id", playerID))

	cursor, err := r.txColl.Find(ctx, bson.M{"player_id": playerID})
	if err != nil {
		l.Error("failed to find transactions", zap.Error(err))
		return nil, err
	}
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			l.Error("cursor close failed", zap.Error(err))
		}
	}()

	var dtos []dto.Transaction
	if err := cursor.All(ctx, &dtos); err != nil {
		l.Error("failed to decode transactions", zap.Error(err))
		return nil, err
	}

	result := make([]*model.Transaction, len(dtos))
	for i, d := range dtos {
		result[i] = txFromDTO(d)
	}
	return result, nil
}
