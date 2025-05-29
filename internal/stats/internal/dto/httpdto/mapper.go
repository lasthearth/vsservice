package httpdto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongo"
	"github.com/lasthearth/vsservice/internal/stats/internal/dto/mongodto"
	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

func (h *Stats) ToMongoDTO() *mongodto.Stats {
	seedStats := make([]mongodto.SeedStats, 0, 1)
	deaths := lo.Map(h.Deaths, func(item Death, index int) mongodto.Death {
		return mongodto.Death{
			Cause:      item.Cause,
			EntityName: item.EntityName,
		}
	})

	now := time.Now()
	return &mongodto.Stats{
		Model: mongo.Model{
			Id:        primitive.NewObjectIDFromTimestamp(now),
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: h.Name,
		SeedStats: append(seedStats, mongodto.SeedStats{
			Seed:          int(h.Seed),
			DeathCount:    h.DeathCount,
			Deaths:        deaths,
			HoursPlayed:   h.HoursPlayed,
			PlayersKilled: h.PlayersKilled,
		}),
		CreatedAt:  now,
		UpdatedAt:  now,
		LastOnline: h.LastOnline,
	}
}
