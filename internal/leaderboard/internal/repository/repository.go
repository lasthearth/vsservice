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
			{"$group",
				bson.D{
					{"_id", "$user_game_name"},
					{"total_hours", bson.D{{"$sum", "$hours_played"}}},
					{"total_deaths", bson.D{{"$sum", "$death_count"}}},
					{"total_kills", bson.D{{"$sum", "$players_killed"}}},
				},
			},
		},
		bson.D{
			{"$project",
				bson.D{
					{"_id", 0},
					{"user_game_name", "$_id"},
					{"total_hours", 1},
					{"total_deaths", 1},
					{"total_kills", 1},
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
