package repository

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/notification/internal/dto"
	"github.com/lasthearth/vsservice/internal/notification/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

func (r *Repository) Create(ctx context.Context, notification model.Notification) error {
	l := r.log.
		WithMethod("create_notification").
		With(zap.String("user_id", notification.UserID))

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

func (r *Repository) ListNotifications(ctx context.Context, limit int, userID, nextPageToken string) ([]model.Notification, error) {
	l := r.log.
		WithMethod("list_notifications").
		With(
			zap.String("user_id", userID),
			zap.String("next_page_token", nextPageToken),
			zap.Int("limit", limit),
		)

	l.Info("listing notifications")

	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	var filter bson.M

	if nextPageToken == "" {
		filter = bson.M{"user_id": userID}
	} else {
		filter = bson.M{
			"_id":     bson.M{"$lt": nextPageToken},
			"user_id": userID,
		}
	}

	sort := bson.D{{
		Key:   "_id",
		Value: -1,
	}}

	opts := options.Find().SetSort(sort).SetLimit(int64(limit))
	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		// TODO: error system
		l.Error("find error", zap.Error(err))
		return nil, err
	}
	defer cursor.Close(ctx)

	var notifications []dto.Notification
	if err := cursor.All(ctx, &notifications); err != nil {
		l.Error("cursor decode error", zap.Error(err))
		return nil, err
	}

	return r.mapper.ToModels(notifications), nil
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

	if _, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, update); err != nil {
		l.Error("update error", zap.Error(err))
		return err
	}

	return nil
}
