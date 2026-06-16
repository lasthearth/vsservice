package repository

import (
	"context"
	"errors"

	mongox "github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/pkg/mongox/orderby"
	"github.com/lasthearth/vsservice/internal/pkg/mongox/pagination"
	favorlogdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/imperial_favor_log"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"
)

func (r *Repository) CreateFavorLog(ctx context.Context, log model.ImperialFavorLog) error {
	l := r.log.WithMethod("CreateFavorLog").
		With(zap.String("settlement_id", log.SettlementId), zap.Int64("amount", log.Amount))

	oid, err := mongox.ParseObjectID(log.SettlementId)
	if err != nil {
		l.Error("invalid settlement id", zap.Error(err))
		return err
	}

	dto := favorlogdto.ImperialFavorLog{
		Model:        mongox.NewModel(),
		SettlementId: oid,
		AdminId:      log.AdminId,
		Amount:       log.Amount,
		Reason:       log.Reason,
	}

	if _, err := r.favorLogColl.InsertOne(ctx, dto); err != nil {
		l.Error("failed to insert favor log", zap.Error(err))
		return err
	}

	return nil
}

var favorLogAllowedSortFields = map[string]string{
	"created_at": "created_at",
	"amount":     "amount",
}

var favorLogDefaultOrder = &orderby.Info{
	Field:      "created_at",
	Direction:  orderby.Desc,
	MongoField: "created_at",
}

func (r *Repository) ListFavorLogs(ctx context.Context, settlementID, adminID, orderByStr, nextToken string) ([]model.ImperialFavorLog, string, error) {
	l := r.log.WithMethod("ListFavorLogs").With(zap.String("settlement_id", settlementID))

	oid, err := mongox.ParseObjectID(settlementID)
	if err != nil {
		l.Error("invalid settlement id", zap.Error(err))
		return nil, "", err
	}

	filter := bson.M{"settlement_id": oid}
	if adminID != "" {
		filter["admin_id"] = adminID
	}

	orderInfo, err := orderby.Parse(orderByStr, favorLogAllowedSortFields, favorLogDefaultOrder)
	if err != nil {
		l.Error("invalid order_by", zap.Error(err))
		return nil, "", err
	}

	sort := orderby.BuildSortOptions(orderInfo)

	opts := []pagination.OptionFn{
		pagination.WithFilter(filter),
		pagination.WithSort(sort),
	}
	if nextToken != "" {
		opts = append(opts, pagination.WithNext(nextToken))
	}

	resp, err := pagination.Find[favorlogdto.ImperialFavorLog](ctx, r.favorLogColl, opts...)
	if err != nil {
		if errors.Is(err, pagination.ErrNoData) {
			return nil, "", nil
		}
		l.Error("failed to list favor logs", zap.Error(err))
		return nil, "", err
	}

	logs := make([]model.ImperialFavorLog, len(resp.Data))
	for i, d := range resp.Data {
		logs[i] = model.ImperialFavorLog{
			Id:           d.Model.Id.Hex(),
			SettlementId: d.SettlementId.Hex(),
			AdminId:      d.AdminId,
			Amount:       d.Amount,
			Reason:       d.Reason,
			CreatedAt:    d.CreatedAt,
		}
	}

	return logs, resp.Next, nil
}
