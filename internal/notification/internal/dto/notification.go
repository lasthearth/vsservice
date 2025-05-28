package dto

import (
	"github.com/lasthearth/vsservice/internal/notification/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongo"
)

// goverter:converter
// goverter:output:file mapper/notification.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:ObjectIdToString
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTime
type NotificationMapper interface {
	FromModels([]model.Notification) []Notification
	// goverter:ignore Model
	FromModel(model.Notification) Notification

	ToModels(dto []Notification) []model.Notification
	// goverter:autoMap Model
	ToModel(dto Notification) model.Notification
}

type Notification struct {
	mongo.Model `bson:",inline"`
	UserID      string `json:"user_id"`
	Title       string `json:"title"`
	Message     string `json:"message"`
	State       string `json:"state"`
}
