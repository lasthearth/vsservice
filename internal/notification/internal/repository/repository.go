package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/lasthearth/vsservice/internal/notification/internal/dto"
	"github.com/lasthearth/vsservice/internal/notification/internal/model"
	"github.com/lasthearth/vsservice/internal/notification/internal/repository/orderby"
	"github.com/lasthearth/vsservice/internal/notification/internal/repository/repoerr"
	"github.com/lasthearth/vsservice/internal/pkg/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
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

	orderInfo, err := orderby.ParseOrderBy(orderBy, allowedSortFields)
	if err != nil {
		l.Error("failed to parse order_by", zap.Error(err))
		return nil, "", fmt.Errorf("invalid order_by: %w", err)
	}

	l.Debug("parsed order_by",
		zap.String("field", orderInfo.Field),
		zap.Int("direction", orderInfo.Direction),
		zap.String("mongo_field", orderInfo.MongoField),
	)

	filter, err := buildPaginationFilter(userID, nextPageToken)
	if err != nil {
		l.Error("failed to build pagination filter", zap.Error(err))
		return nil, "", fmt.Errorf("failed to build pagination filter: %w", err)
	}

	sort := orderby.BuildSortOptions(orderInfo)

	opts := options.Find().SetSort(sort).SetLimit(int64(limit))
	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		// TODO: error system
		l.Error("find error", zap.Error(err))
		return nil, "", err
	}
	defer cursor.Close(ctx)

	var notifications []dto.Notification
	if err := cursor.All(ctx, &notifications); err != nil {
		l.Error("cursor decode error", zap.Error(err))
		return nil, "", err
	}

	l.Debug("fetched notifications", zap.Int("count", len(notifications)))

	models := r.mapper.ToModels(notifications)

	if len(models) == 0 {
		return nil, "", repoerr.ErrNotificationsNotFound
	}

	next := ""
	if len(models) == limit {
		next = models[len(models)-1].Id
	}

	return models, next, nil
}

func buildPaginationFilter(userID, nextPageToken string) (bson.M, error) {
	baseFilter := bson.M{"user_id": userID}

	if nextPageToken == "" {
		return baseFilter, nil
	}

	oid, err := mongo.ParseObjectID(nextPageToken)
	if err != nil {
		return nil, fmt.Errorf("invalid page token: %w", err)
	}

	baseFilter["_id"] = bson.M{"$lt": oid}
	return baseFilter, nil
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
