package dto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type News struct {
	Model     mongox.Model `bson:",inline"`
	Title     string       `bson:"title"`
	Preview   string       `bson:"preview"`
	Content   string       `bson:"content,omitempty"`
	ViewCount int64        `bson:"view_count"`
}

func (n News) Id() bson.ObjectID {
	return n.Model.Id
}
