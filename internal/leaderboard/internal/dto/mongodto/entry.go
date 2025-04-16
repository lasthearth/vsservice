package mongodto

import "go.mongodb.org/mongo-driver/bson/primitive"

type Entry struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	Name        string             `bson:"name"`
	DeathCount  int                `bson:"death_count"`
	KillCount   int                `bson:"kill_count"`
	HoursPlayed int                `bson:"hours_played,truncate"`
}
