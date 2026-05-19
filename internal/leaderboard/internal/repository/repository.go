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
	return r.listEntries(ctx, "total_deaths", limit)
}

func (r *Repository) ListEntriesSortByKills(ctx context.Context, limit int) ([]*model.Entry, error) {
	return r.listEntries(ctx, "total_kills", limit)
}

func (r *Repository) ListEntriesSortByOnline(ctx context.Context, limit int) ([]*model.Entry, error) {
	return r.listEntries(ctx, "total_hours", limit)
}

func (r *Repository) listEntries(
	ctx context.Context,
	filter string,
	limit int,
) ([]*model.Entry, error) {
	pipeline := bson.A{
		bson.D{
			{Key: "$group", Value: bson.D{
				{Key: "_id", Value: "$user_game_name"},
				{Key: "total_hours", Value: bson.D{{Key: "$sum", Value: "$hours_played"}}},
				{Key: "total_deaths", Value: bson.D{{Key: "$sum", Value: "$death_count"}}},
				{Key: "total_kills", Value: bson.D{{Key: "$sum", Value: "$players_killed"}}},
			}},
		},
		bson.D{
			{Key: "$project", Value: bson.D{
				{Key: "_id", Value: 0},
				{Key: "user_game_name", Value: "$_id"},
				{Key: "total_hours", Value: 1},
				{Key: "total_deaths", Value: 1},
				{Key: "total_kills", Value: 1},
			}},
		},
		bson.D{{Key: "$sort", Value: bson.D{{Key: filter, Value: -1}}}},
		bson.D{{Key: "$limit", Value: limit}},
	}
	cursor, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		r.log.Error("aggregation error", zap.Error(err))
		return nil, err
	}
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			r.log.Error("cursor close failed", zap.Error(err))
		}
	}()

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
			UserId:      user.UserID,
			Name:        item.Name,
			TotalHours:  item.TotalHours,
			TotalDeaths: item.TotalDeaths,
			TotalKills:  item.TotalKills,
		}
	})

	return entries, nil
}
