package mongo

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Model struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	CreatedAt time.Time          `bson:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at"`
}

func NewModel() Model {
	now := time.Now()
	return Model{
		ID:        primitive.NewObjectIDFromTimestamp(now),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func ParseObjectID(id string) (primitive.ObjectID, error) {
	return primitive.ObjectIDFromHex(id)
}
