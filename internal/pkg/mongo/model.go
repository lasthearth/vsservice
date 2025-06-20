package mongo

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type Model struct {
	Id        bson.ObjectID `bson:"_id,omitempty"`
	CreatedAt time.Time     `bson:"created_at"`
	UpdatedAt time.Time     `bson:"updated_at"`
}

func NewModel() Model {
	now := time.Now()
	return Model{
		Id:        bson.NewObjectIDFromTimestamp(now),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func ParseObjectID(id string) (primitive.ObjectID, error) {
	return primitive.ObjectIDFromHex(id)
}
