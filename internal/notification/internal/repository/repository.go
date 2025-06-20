package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/lasthearth/vsservice/internal/notification/internal/dto"
	"github.com/lasthearth/vsservice/internal/notification/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongo"
	"github.com/lasthearth/vsservice/internal/pkg/mongo/orderby"
	"github.com/lasthearth/vsservice/internal/pkg/mongo/pagination"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"
)

// goverter:converter
// goverter:output:file repomapper/notification.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:ObjectIdToString
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTime
type NotificationMapper interface {
	FromModels([]model.Notification) []dto.Notification
	// goverter:ignore Model
	FromModel(model.Notification) dto.Notification

	ToModels(dto []dto.Notification) []model.Notification
	// goverter:autoMap Model
	ToModel(dto dto.Notification) model.Notification
}

func (r *Repository) Create(ctx context.Context, notification model.Notification) error {
	l := r.log.
		WithMethod("create_notification").
		With(zap.String("user_id", notification.UserId))

	l.Info("creating notification")

	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	dto := r.mapper.FromModel(notification)

	dto.Model = mongo.NewModel()

	if _, err := r.coll.InsertOne(ctx, dto); err != nil {
		l.Error("insert error", zap.Error(err))
		return err
	}

	return nil
}

func (r *Repository) ListNotifications(ctx context.Context, limit int, userID, nextPageToken, orderBy string) ([]model.Notification, string, error) {
	l := r.log.
		WithMethod("list_notifications").
		With(
			zap.String("user_id", userID),
			zap.String("next_page_token", nextPageToken),
			zap.Int("limit", limit),
		)

	allowedSortFields := map[string]string{
		"created_at": "created_at",
	}

	l.Info("listing notifications")

	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	orderInfo, err := orderby.Parse(
		orderBy,
		allowedSortFields,
		&orderby.OrderByInfo{
			Field:      "created_at",
			Direction:  orderby.Desc,
			MongoField: "created_at",
		},
	)
	if err != nil {
		l.Error("failed to parse order_by", zap.Error(err))
		return nil, "", fmt.Errorf("invalid order_by: %w", err)
	}

	l.Debug("parsed order_by",
		zap.String("field", orderInfo.Field),
		zap.Int("direction", int(orderInfo.Direction)),
		zap.String("mongo_field", orderInfo.MongoField),
	)

	filter := bson.M{
		"$or": bson.A{
			bson.M{"user_id": userID},
			bson.M{"user_id": model.BroadcastUserId},
		},
	}
	sort := orderby.BuildSortOptions(orderInfo)

	resp, err := pagination.Find[dto.Notification](
		ctx,
		r.coll,
		pagination.WithFilter(filter),
		pagination.WithSort(sort),
		pagination.WithLimit(int64(limit)),
	)
	if err != nil {
		l.Error("failed to find notifications", zap.Error(err))
		return nil, "", fmt.Errorf("failed to find notifications: %w", err)
	}

	return r.mapper.ToModels(resp.Data), resp.Next, nil
}

func (r *Repository) MarkNotificationRead(ctx context.Context, id string) error {
	l := r.log.
		WithMethod("mark_notification_read").
		With(zap.String("id", id))

	l.Info("marking notification read")

	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"state": model.NotificationStateRead,
		},
	}

	oid, err := mongo.ParseObjectID(id)
	if err != nil {
		l.Error("parse object id error", zap.Error(err))
		return err
	}

	if _, err := r.coll.UpdateOne(ctx, bson.M{"_id": oid}, update); err != nil {
		l.Error("update error", zap.Error(err))
		return err
	}

	return nil
}
