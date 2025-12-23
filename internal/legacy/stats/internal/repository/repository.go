package repository

import (
	"context"
	"time"

	"github.com/go-faster/errors"
	"github.com/lasthearth/vsservice/internal/stats/internal/dto/httpdto"
	"github.com/lasthearth/vsservice/internal/stats/internal/dto/mongodto"
	"github.com/lasthearth/vsservice/internal/stats/internal/model"
	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

func (r *Repository) GetByName(ctx context.Context, name string) (*model.Stats, error) {
	var stats mongodto.Stats
	find := r.coll.FindOne(ctx, bson.M{"name": name})
	err := find.Decode(&stats)
	if err != nil {
		r.log.Error("failed to get by name", zap.Error(err))
		return nil, ErrNotFound
	}

	return stats.ToModel(), nil
}

func (r *Repository) Exists(ctx context.Context, name string) (bool, error) {
	count, err := r.coll.CountDocuments(
		ctx,
		bson.M{
			"name": name,
		},
	)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return false, nil
		}

		r.log.Error("exist failed", zap.Error(err))
		return false, ErrNotFound
	}

	return count > 0, nil
}

func (r *Repository) Create(ctx context.Context, httpStats *httpdto.Stats) (*model.Stats, error) {
	dto := httpStats.ToMongoDTO()
	_, err := r.coll.InsertOne(ctx, dto)
	if err != nil {
		return nil, ErrCreate
	}

	return r.GetByName(ctx, httpStats.Name)
}

func (r *Repository) Update(ctx context.Context, stats *httpdto.Stats) (*model.Stats, error) {
	_, err := r.GetByName(ctx, stats.Name)
	if err != nil {
		return nil, err
	}

	deaths := lo.Map(stats.Deaths, func(item httpdto.Death, _ int) mongodto.Death {
		return mongodto.Death{
			Cause:      item.Cause,
			EntityName: item.EntityName,
		}
	})

	now := time.Now()
	seed := int(stats.Seed)

	updateExisting := bson.M{
		"$set": bson.M{
			"last_online":                       stats.LastOnline,
			"updated_at":                        now,
			"seed_stats.$[elem].death_count":    stats.DeathCount,
			"seed_stats.$[elem].deaths":         deaths,
			"seed_stats.$[elem].hours_played":   stats.HoursPlayed,
			"seed_stats.$[elem].players_killed": stats.PlayersKilled,
		},
	}
	arrayFilters := []interface{}{bson.M{"elem.seed": seed}}
	opts := options.UpdateOne().SetArrayFilters(arrayFilters)

	res, err := r.coll.UpdateOne(
		ctx,
		bson.M{"name": stats.Name, "seed_stats.seed": seed},
		updateExisting,
		opts,
	)
	if err != nil {
		r.log.Error("failed to update existing seed stats", zap.Error(err))
		return nil, ErrUpdate
	}

	if res.MatchedCount == 0 {
		newSeedStats := mongodto.SeedStats{
			Seed:          seed,
			DeathCount:    stats.DeathCount,
			Deaths:        deaths,
			HoursPlayed:   stats.HoursPlayed,
			PlayersKilled: stats.PlayersKilled,
		}
		updatePush := bson.M{
			"$set": bson.M{
				"last_online": stats.LastOnline,
				"updated_at":  now,
			},
			"$push": bson.M{
				"seed_stats": newSeedStats,
			},
		}
		_, err = r.coll.UpdateOne(
			ctx,
			bson.M{"name": stats.Name},
			updatePush,
		)
		if err != nil {
			r.log.Error("failed to push new seed stats", zap.Error(err))
			return nil, ErrUpdate
		}
	}

	return r.GetByName(ctx, stats.Name)
}
