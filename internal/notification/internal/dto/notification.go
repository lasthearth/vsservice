package dto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongo"
)

type Notification struct {
	mongo.Model `bson:",inline"`
	UserId      string `bson:"user_id"`
	Title       string `bson:"title"`
	Message     string `bson:"message"`
	State       string `bson:"state"`
}
