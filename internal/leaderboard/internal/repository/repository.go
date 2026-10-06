package repository

import (
	"context"
	"time"

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
				{Key: "last_online", Value: bson.D{{Key: "$max", Value: "$last_online"}}},
			}},
		},
		bson.D{{Key: "$sort", Value: bson.D{{Key: filter, Value: -1}}}},
		bson.D{{Key: "$limit", Value: limit}},
		// One join instead of a FindOne per entry: the site asks for every
		// player at once, which used to cost one round-trip per row.
		bson.D{
			{Key: "$lookup", Value: bson.D{
				{Key: "from", Value: playerCollName},
				{Key: "localField", Value: "_id"},
				{Key: "foreignField", Value: "user_game_name"},
				{Key: "as", Value: "player"},
			}},
		},
		bson.D{
			{Key: "$project", Value: bson.D{
				{Key: "_id", Value: 0},
				{Key: "user_game_name", Value: "$_id"},
				{Key: "total_hours", Value: 1},
				{Key: "total_deaths", Value: 1},
				{Key: "total_kills", Value: 1},
				{Key: "last_online", Value: 1},
				{Key: "user_id", Value: bson.D{{Key: "$arrayElemAt", Value: bson.A{"$player.user_id", 0}}}},
			}},
		},
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

	entries := lo.Map(rawEntries, func(item *mongodto.Entry, _ int) *model.Entry {
		return &model.Entry{
			UserId:      item.UserId,
			Name:        item.Name,
			TotalHours:  item.TotalHours,
			TotalDeaths: item.TotalDeaths,
			TotalKills:  item.TotalKills,
			LastOnline:  lastOnline(item.LastOnline),
		}
	})

	return entries, nil
}

// lastOnline reads the game-written last-online mark whatever BSON type it
// was stored as: a date, an RFC 3339 string, or unix seconds/milliseconds.
// Zero or unreadable values mean "unknown" and yield nil.
func lastOnline(raw bson.RawValue) *time.Time {
	var t time.Time

	switch raw.Type {
	case bson.TypeDateTime:
		t = raw.Time()
	case bson.TypeString:
		parsed, err := time.Parse(time.RFC3339Nano, raw.StringValue())
		if err != nil {
			return nil
		}
		t = parsed
	case bson.TypeInt64, bson.TypeInt32, bson.TypeDouble:
		n, ok := raw.AsInt64OK()
		if !ok {
			return nil
		}
		if n > 1e11 {
			t = time.UnixMilli(n)
		} else {
			t = time.Unix(n, 0)
		}
	default:
		return nil
	}

	if t.IsZero() || t.Year() < 2000 {
		return nil
	}

	t = t.UTC()
	return &t
}
