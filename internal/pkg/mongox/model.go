package mongox

import (
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Model struct {
	Id        bson.ObjectID `bson:"_id,omitempty"`
	CreatedAt time.Time     `bson:"created_at"`
	UpdatedAt time.Time     `bson:"updated_at"`
	// Version increments on every UpdateDoc write and is what pins an update to
	// the state it read. updated_at cannot do that alone: it is truncated to a
	// millisecond, so two writes landing in the same millisecond leave it
	// unchanged and a third writer's guard still matches the value it loaded —
	// an ABA that lets a stale write through. A counter only ever moves forward.
	//
	// Documents written before this field existed decode as 0, which the guard
	// treats as "missing or zero", so no backfill is needed.
	Version int64 `bson:"version"`
}

func NewModel() Model {
	now := time.Now()
	return Model{
		Id:        bson.NewObjectIDFromTimestamp(now),
		CreatedAt: now,
		UpdatedAt: now,
		Version:   1,
	}
}

func ParseObjectID(id string) (bson.ObjectID, error) {
	return bson.ObjectIDFromHex(id)
}

func ParseAnyObjectID(id any) (bson.ObjectID, error) {
	switch v := id.(type) {
	case string:
		return ParseObjectID(v)
	case bson.ObjectID:
		return v, nil
	default:
		return bson.ObjectID{}, errors.New("invalid object id")
	}
}
