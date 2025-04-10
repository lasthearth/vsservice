package repository

import (
	"context"
	"github.com/ripls56/vsservice/internal/leaderboard/internal/dto/mongodto"
	"github.com/ripls56/vsservice/internal/leaderboard/internal/model"
	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"
)

func (r *Repository) ListEntriesSortByDeath(ctx context.Context, limit int) ([]*model.Entry, error) {
	return r.listEntries(ctx, "death_count", limit)
}

func (r *Repository) ListEntriesSortByKills(ctx context.Context, limit int) ([]*model.Entry, error) {
	return r.listEntries(ctx, "kill_count", limit)
}

func (r *Repository) ListEntriesSortByOnline(ctx context.Context, limit int) ([]*model.Entry, error) {
	return r.listEntries(ctx, "hours_played", limit)
}

func (r *Repository) listEntries(
	ctx context.Context,
	filter string,
	limit int,
) ([]*model.Entry, error) {
	pipeline := bson.A{
		bson.D{{"$unwind", bson.D{{"path", "$seed_stats"}}}},
		bson.D{
			{"$group",
				bson.D{
					{"_id", "$_id"},
					{"name", bson.D{{"$first", "$name"}}},
					{"death_count", bson.D{{"$sum", "$seed_stats.death_count"}}},
					{"kill_count", bson.D{{"$sum", "$seed_stats.players_killed"}}},
					{"hours_played", bson.D{{"$sum", "$seed_stats.hours_played"}}},
				},
			},
		},
		bson.D{{"$sort", bson.D{{filter, -1}}}},
		bson.D{{"$limit", limit}},
	}
	cursor, err := r.coll.Aggregate(ctx, pipeline)

	if err != nil {
		r.log.Error("aggregation error", zap.Error(err))
		return nil, err
	}
	defer cursor.Close(ctx)

	var rawEntries []*mongodto.Entry
	if err = cursor.All(ctx, &rawEntries); err != nil {
		return nil, err
	}

	entries := lo.Map(rawEntries, func(item *mongodto.Entry, index int) *model.Entry {
		return &model.Entry{
			Name:        item.Name,
			DeathCount:  item.DeathCount,
			KillCount:   item.KillCount,
			HoursPlayed: float32(item.HoursPlayed),
		}
	})

	return entries, nil
}
