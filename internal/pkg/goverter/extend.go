package goverter

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func ObjectIdToString(id primitive.ObjectID) string {
	return id.Hex()
}

func StringToObjectID(id string) (primitive.ObjectID, error) {
	return primitive.ObjectIDFromHex(id)
}

func TimeToTime(t time.Time) time.Time {
	return t
}
