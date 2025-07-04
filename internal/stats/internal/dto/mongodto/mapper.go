package mongodto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongo"
	"github.com/lasthearth/vsservice/internal/stats/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func (m *Stats) ToModel() *model.Stats {
	var stats model.Stats
	seeds := make([]int, len(m.SeedStats))
	for _, seed := range m.SeedStats {
		seeds = append(seeds, seed.Seed)
		stats.DeathCount += seed.DeathCount
		stats.HoursPlayed += seed.HoursPlayed
		stats.PlayersKilled += seed.PlayersKilled
	}

	return &model.Stats{
		ID:            m.Id.String(),
		Name:          m.Name,
		DeathCount:    stats.DeathCount,
		Seeds:         seeds,
		HoursPlayed:   stats.HoursPlayed,
		LastOnline:    time.UnixMilli(m.LastOnline),
		PlayersKilled: stats.PlayersKilled,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}

func FromModel(stats *model.Stats) *Stats {
	seedStats := SeedStats{
		DeathCount:    stats.DeathCount,
		HoursPlayed:   stats.HoursPlayed,
		PlayersKilled: stats.PlayersKilled,
	}

	now := time.Now()

	return &Stats{
		Model: mongo.Model{
			Id:        bson.NewObjectIDFromTimestamp(now),
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:       stats.Name,
		SeedStats:  []SeedStats{seedStats},
		LastOnline: stats.LastOnline.UnixMilli(),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}
