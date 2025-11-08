package repository

import (
	"context"

	"github.com/lasthearth/vsservice/internal/leaderboard/internal/dto/mongodto"
	"github.com/lasthearth/vsservice/internal/leaderboard/internal/model"
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
		bson.D{{
			Key:   "$unwind",
			Value: bson.D{{Key: "path", Value: "$seed_stats"}},
		}},
		bson.D{
			{
				Key: "$group",
				Value: bson.D{
					{Key: "_id", Value: "$_id"},
					{
						Key:   "name",
						Value: bson.D{{Key: "$first", Value: "$name"}},
					},
					{
						Key: "death_count",
						Value: bson.D{{
							Key:   "$sum",
							Value: "$seed_stats.death_count",
						}},
					},
					{
						Key: "kill_count",
						Value: bson.D{
							{
								Key:   "$sum",
								Value: "$seed_stats.players_killed",
							},
						},
					},
					{
						Key:   "hours_played",
						Value: bson.D{{Key: "$sum", Value: "$seed_stats.hours_played"}},
					},
				},
			},
		},
		bson.D{{
			Key:   "$sort",
			Value: bson.D{{Key: filter, Value: -1}},
		}},
		bson.D{{Key: "$limit", Value: limit}},
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
		user := struct {
			UserID string `bson:"user_id"`
		}{}
		finded := r.pColl.FindOne(ctx, bson.M{"user_game_name": item.Name})
		if err := finded.Err(); err != nil {
			r.log.Error("find user error", zap.Error(err))
		}
		err = finded.Decode(&user)
		if err != nil {
			r.log.Error("decode user error", zap.Error(err))
		}

		return &model.Entry{
			Name:        item.Name,
			DeathCount:  item.DeathCount,
			KillCount:   item.KillCount,
			HoursPlayed: float32(item.HoursPlayed),
			UserID:      user.UserID,
		}
	})

	return entries, nil
}
