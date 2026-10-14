package goverter

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func ObjectIdToObjectId(id bson.ObjectID) bson.ObjectID {
	return id
}

func ObjectIdToString(id bson.ObjectID) string {
	return id.Hex()
}

func StringToObjectID(id string) (bson.ObjectID, error) {
	return bson.ObjectIDFromHex(id)
}

func IntToInt32(i int) int32 {
	return int32(i)
}

func IntToInt64(i int) int64 {
	return int64(i)
}

func TimeToTime(t time.Time) time.Time {
	return t
}

func TimestampToTime(t *timestamppb.Timestamp) time.Time {
	return t.AsTime()
}

func TimeToTimestamp(t time.Time) *timestamppb.Timestamp {
	return timestamppb.New(t)
}

// TimePtrToTimestamp converts an optional time. A time a protobuf Timestamp
// cannot carry becomes nil instead: protojson refuses to marshal seconds
// outside 0001-01-01..9999-12-31, so one corrupt value in a collection this
// service does not write would otherwise fail the whole response rather than
// that one field. The leaderboard guards its last_online for the same reason.
func TimePtrToTimestamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil || t.Year() < 1 || t.Year() > 9999 {
		return nil
	}
	return timestamppb.New(*t)
}

func TimeToInt64(t time.Time) int64 {
	return t.Unix()
}
