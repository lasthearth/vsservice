package mongodto

import "go.mongodb.org/mongo-driver/v2/bson"

type Entry struct {
	ID          bson.ObjectID `bson:"_id,omitempty"`
	Name        string        `bson:"name"`
	DeathCount  int           `bson:"death_count"`
	KillCount   int           `bson:"kill_count"`
	HoursPlayed int           `bson:"hours_played,truncate"`
}
