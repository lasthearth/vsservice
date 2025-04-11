package mongodto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongo"
	"time"
)

type Stats struct {
	mongo.Model `bson:",inline"`
	Name        string      `bson:"name"`
	SeedStats   []SeedStats `bson:"seed_stats"`
	LastOnline  int64       `bson:"last_online,truncate"`
	CreatedAt   time.Time   `bson:"created_at"`
	UpdatedAt   time.Time   `bson:"updated_at"`
}

type SeedStats struct {
	Seed          int     `bson:"seed,truncate"`
	DeathCount    int     `bson:"death_count,truncate"`
	Deaths        []Death `bson:"deaths"`
	HoursPlayed   float32 `bson:"hours_played"`
	PlayersKilled int     `bson:"players_killed,truncate"`
}
