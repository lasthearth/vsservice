package dto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type Notification struct {
	mongox.Model `bson:",inline"`
	UserId       string `bson:"user_id"`
	Title        string `bson:"title"`
	Message      string `bson:"message"`
	State        string `bson:"state"`
}

func (n Notification) Id() bson.ObjectID {
	return n.Model.Id
}
