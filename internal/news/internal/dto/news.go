package dto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type News struct {
	Model   mongo.Model `bson:",inline"`
	Title   string      `bson:"title"`
	Preview string      `bson:"preview"`
	Content string      `bson:"content,omitempty"`
}

func (n News) Id() bson.ObjectID {
	return n.Model.Id
}
