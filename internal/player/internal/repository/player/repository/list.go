package repository

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/regexsafe"
	dto "github.com/lasthearth/vsservice/internal/player/internal/dto/mongo"
	"github.com/lasthearth/vsservice/internal/player/internal/model"
	"github.com/lasthearth/vsservice/internal/player/internal/model/verification"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"
)

// The windows the roster filters on. A window is measured against the request's
// own clock (the now argument), never against time.Now, so the boundaries a
// test sees are the boundaries the pipeline carries.
const (
	windowDay   = 24 * time.Hour
	windowWeek  = 7 * 24 * time.Hour
	windowMonth = 30 * 24 * time.Hour
)

// nicknameSortKey is the lowercased game name the roster sorts by. It stands in
// for an aggregation collation, which cannot be limited to the sort alone: see
// ListPlayers.
const nicknameSortKey = "nickname_key"

// ListPlayers answers the public roster page.
//
// Hours and last_online are summed over the player's stats documents exactly the
// way the leaderboard sums them, so a player's roster row and leaderboard row
// never disagree. The page is numbered rather than cursor-based because the site
// shows "page N of M" with a count on every filter, which a cursor cannot
// answer: the roster skips through both remaining stages once per request.
//
// The numbers this rests on are small — a few hundred players — so $skip is
// cheap. If the roster ever passes ~10k, hours and last_online move into the
// player document and this lookup goes away; until then one aggregate is the
// only way to keep the two pages on the same numbers.
func (r *Repository) ListPlayers(
	ctx context.Context,
	filter model.PlayerListFilter,
	page, size int,
	now time.Time,
) (*model.PlayerListPage, error) {
	l := r.log.WithMethod("list_players")

	// No aggregation-wide collation, deliberately. A collation also governs the
	// $lookup equality, so two players whose game names differ only in case would
	// join the same stats and share one another's hours, disagreeing with the
	// leaderboard, which groups this collection without one. It would also make
	// both user_game_name indexes unusable, because an index only serves an
	// operation whose collation matches its own. The case-insensitive nickname
	// order the site asks for comes from a lowercased sort key instead.
	cursor, err := r.coll.Aggregate(ctx, buildListPlayersPipeline(filter, page, size, now))
	if err != nil {
		l.Error("aggregation error", zap.Error(err))
		return nil, err
	}
	defer func() {
		if cerr := cursor.Close(ctx); cerr != nil {
			l.Error("cursor close failed", zap.Error(cerr))
		}
	}()

	var out []listPlayersFacet
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}

	players := []model.PlayerListItem{}
	counts := model.LastSeenCounts{}
	var total int32

	if len(out) > 0 {
		facet := out[0]
		players = make([]model.PlayerListItem, 0, len(facet.Items))
		prefix := r.getAvatarPrefix()

		for _, row := range facet.Items {
			players = append(players, model.PlayerListItem{
				UserId:      row.UserId,
				Nickname:    row.Nickname,
				Avatar:      AvatarWithPrefix(row.Avatar, prefix),
				IsOnline:    row.IsOnline,
				LastOnline:  row.LastOnline,
				HoursPlayed: row.Hours,
				About:       row.About,
			})
		}

		if len(facet.Counts) > 0 {
			counts = model.LastSeenCounts{
				All:      facet.Counts[0].All,
				Online:   facet.Counts[0].Online,
				Day:      facet.Counts[0].Day,
				Week:     facet.Counts[0].Week,
				Month:    facet.Counts[0].Month,
				Inactive: facet.Counts[0].Inactive,
			}
		}

		if len(facet.Total) > 0 {
			total = facet.Total[0].Total
		}
	}

	return &model.PlayerListPage{
		Players:   players,
		TotalSize: total,
		Page:      int32(page),
		PageSize:  int32(size),
		Counts:    counts,
	}, nil
}

// listPlayersFacet is what the $facet stage hands back: the counts for every
// window, the size of the filtered roster, and one page of rows.
type listPlayersFacet struct {
	Counts []listPlayersCounts `bson:"counts"`
	Total  []listPlayersTotal  `bson:"total"`
	Items  []listPlayersRow    `bson:"items"`
}

type listPlayersCounts struct {
	All      int32 `bson:"all"`
	Online   int32 `bson:"online"`
	Day      int32 `bson:"day"`
	Week     int32 `bson:"week"`
	Month    int32 `bson:"month"`
	Inactive int32 `bson:"inactive"`
}

type listPlayersTotal struct {
	Total int32 `bson:"total"`
}

// listPlayersRow is one projected row. last_online arrives as the unix
// milliseconds NumberLong the game writes, which the driver decodes into a time.
type listPlayersRow struct {
	UserId     string      `bson:"user_id"`
	Nickname   string      `bson:"nickname"`
	Avatar     *dto.Avatar `bson:"avatar"`
	IsOnline   bool        `bson:"is_online"`
	LastOnline *time.Time  `bson:"last_online"`
	Hours      float64     `bson:"hours_played"`
	About      string      `bson:"about"`
}

// buildListPlayersPipeline is the whole roster query. It is kept free of the
// database so its escaping, window boundaries, ordering and paging can be read
// and tested on their own.
func buildListPlayersPipeline(
	filter model.PlayerListFilter,
	page, size int,
	now time.Time,
) bson.A {
	match := bson.M{
		// A verification request with no game name cannot be joined to stats and
		// cannot be addressed by nickname, so it is not a roster row.
		"user_game_name": bson.M{"$nin": bson.A{"", nil}},
		"status": bson.M{"$in": bson.A{
			string(verification.VerificationStatusVerified),
			string(verification.VerificationStatusApproved),
		}},
	}

	if filter.Query != "" {
		// The query goes into the pattern escaped: without it a caller could
		// build a regex, and SearchUsers already shows what that costs.
		pattern := bson.M{"$regex": regexsafe.Escape(filter.Query), "$options": "i"}
		match["$or"] = bson.A{
			bson.M{"user_game_name": pattern},
			bson.M{"previous_nickname": pattern},
		}
	}

	daySince := now.Add(-windowDay).UnixMilli()
	weekSince := now.Add(-windowWeek).UnixMilli()
	monthSince := now.Add(-windowMonth).UnixMilli()

	window := windowMatch(filter.LastSeen, now)

	return bson.A{
		bson.D{{Key: "$match", Value: match}},
		// One join instead of a read per row, and grouped so the sum and the
		// maximum come from the same place the leaderboard takes them.
		//
		// The join condition is an $expr inside the sub-pipeline rather than
		// localField and foreignField: the manual names those two and pipeline as
		// mutually exclusive. A server that starts enforcing that would fail every
		// roster request, and the tests here, which read the built bson.A rather
		// than run it, would stay green.
		bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: statsCollName},
			{Key: "let", Value: bson.D{{Key: "gameName", Value: "$user_game_name"}}},
			{Key: "pipeline", Value: bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{
					{Key: "$eq", Value: bson.A{"$user_game_name", "$$gameName"}},
				}}}}},
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: nil},
					{Key: "hours", Value: bson.D{{Key: "$sum", Value: "$hours_played"}}},
					{Key: "last_online", Value: bson.D{{Key: "$max", Value: "$last_online"}}},
				}}},
			}},
			{Key: "as", Value: "player_stats"},
		}}},
		bson.D{{Key: "$addFields", Value: bson.D{
			{Key: "hours", Value: bson.D{{Key: "$ifNull", Value: bson.A{
				bson.D{{Key: "$arrayElemAt", Value: bson.A{"$player_stats.hours", 0}}},
				0,
			}}}},
			{Key: "last_online", Value: bson.D{{Key: "$arrayElemAt", Value: bson.A{
				"$player_stats.last_online", 0,
			}}}},
			{Key: "has_stats", Value: bson.D{{Key: "$gt", Value: bson.A{
				bson.D{{Key: "$size", Value: "$player_stats"}}, 0,
			}}}},
			{Key: nicknameSortKey, Value: bson.D{{Key: "$toLower", Value: "$user_game_name"}}},
		}}},
		// An approved application the game never saw is not a player yet: it
		// stays out of the roster until it has stats or the code is entered.
		bson.D{{Key: "$match", Value: bson.M{"$or": bson.A{
			bson.M{"status": string(verification.VerificationStatusVerified)},
			bson.M{"has_stats": true},
		}}}},
		bson.D{{Key: "$facet", Value: bson.D{
			{Key: "counts", Value: bson.A{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: nil},
					{Key: "all", Value: bson.D{{Key: "$sum", Value: 1}}},
					{Key: "online", Value: countCond(onlineExpr())},
					{Key: "day", Value: countCond(bson.D{{Key: "$or", Value: bson.A{
						onlineExpr(), seenSinceExpr(daySince),
					}}})},
					{Key: "week", Value: countCond(bson.D{{Key: "$or", Value: bson.A{
						onlineExpr(), seenSinceExpr(weekSince),
					}}})},
					{Key: "month", Value: countCond(bson.D{{Key: "$or", Value: bson.A{
						onlineExpr(), seenSinceExpr(monthSince),
					}}})},
					{Key: "inactive", Value: countCond(inactiveExpr(monthSince))},
				}}},
			}},
			{Key: "total", Value: bson.A{
				bson.D{{Key: "$match", Value: window}},
				bson.D{{Key: "$count", Value: "total"}},
			}},
			{Key: "items", Value: bson.A{
				bson.D{{Key: "$match", Value: window}},
				bson.D{{Key: "$sort", Value: sortStage(filter.Sort)}},
				bson.D{{Key: "$skip", Value: int64(skipFor(page, size))}},
				bson.D{{Key: "$limit", Value: int64(size)}},
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "user_id", Value: 1},
					{Key: "nickname", Value: "$user_game_name"},
					{Key: "avatar", Value: 1},
					{Key: "is_online", Value: 1},
					{Key: "last_online", Value: 1},
					{Key: "hours_played", Value: "$hours"},
					{Key: "about", Value: 1},
				}}},
			}},
		}}},
	}
}

// skipFor is the number of rows the requested page leaves behind. Page numbers
// are 1-based; a page below 1 is the first one.
func skipFor(page, size int) int {
	if page < 1 {
		page = 1
	}

	return (page - 1) * size
}

// countCond counts the rows a window condition holds for.
func countCond(cond any) bson.D {
	return bson.D{{Key: "$sum", Value: bson.D{{Key: "$cond", Value: bson.A{cond, 1, 0}}}}}
}

// onlineExpr is true while the game reports the player in game.
func onlineExpr() bson.D {
	return bson.D{{Key: "$eq", Value: bson.A{"$is_online", true}}}
}

// seenSinceExpr is true when the player's last mark is at or after ms.
func seenSinceExpr(ms int64) bson.D {
	return bson.D{{Key: "$gte", Value: bson.A{"$last_online", ms}}}
}

// inactiveExpr is true for a player outside the month window: the game recorded
// a mark long ago, or never recorded one at all.
func inactiveExpr(monthSince int64) bson.D {
	return bson.D{{Key: "$and", Value: bson.A{
		bson.D{{Key: "$ne", Value: bson.A{"$is_online", true}}},
		bson.D{{Key: "$lt", Value: bson.A{
			bson.D{{Key: "$ifNull", Value: bson.A{"$last_online", int64(0)}}},
			monthSince,
		}}},
	}}}
}

// windowMatch is the document-level filter for one window. The windows nest:
// DAY counts everyone ONLINE too, and INACTIVE is everything MONTH leaves out.
func windowMatch(window model.LastSeenWindow, now time.Time) bson.M {
	online := bson.M{"is_online": true}
	seenSince := func(d time.Duration) bson.M {
		return bson.M{"last_online": bson.M{"$gte": now.Add(-d).UnixMilli()}}
	}

	switch window {
	case model.LastSeenOnline:
		return online
	case model.LastSeenDay:
		return bson.M{"$or": bson.A{online, seenSince(windowDay)}}
	case model.LastSeenWeek:
		return bson.M{"$or": bson.A{online, seenSince(windowWeek)}}
	case model.LastSeenMonth:
		return bson.M{"$or": bson.A{online, seenSince(windowMonth)}}
	case model.LastSeenInactive:
		return bson.M{"$and": bson.A{
			bson.M{"is_online": bson.M{"$ne": true}},
			// A player the game never recorded carries no mark at all.
			bson.M{"$or": bson.A{
				bson.M{"last_online": bson.M{"$lt": now.Add(-windowMonth).UnixMilli()}},
				bson.M{"last_online": bson.M{"$exists": false}},
				bson.M{"last_online": nil},
			}},
		}}
	default:
		return bson.M{}
	}
}

// sortStage is the roster order. A descending last_online puts the players the
// game never recorded at the end, which is where "никогда" belongs. Nicknames
// sort on the lowercased key, so the order does not depend on how a player
// capitalised their name.
//
// Every order ends on _id. Without a key that separates every row, $sort is not
// stable, and two players the order ties can land on the same page twice or on
// no page at all while total_size still counts both.
func sortStage(sort model.PlayerListSort) bson.D {
	switch sort {
	case model.PlayerSortHours:
		return bson.D{{Key: "hours", Value: -1}, {Key: nicknameSortKey, Value: 1}, {Key: "_id", Value: 1}}
	case model.PlayerSortNickname:
		return bson.D{{Key: nicknameSortKey, Value: 1}, {Key: "_id", Value: 1}}
	default:
		return bson.D{
			{Key: "is_online", Value: -1},
			{Key: "last_online", Value: -1},
			{Key: nicknameSortKey, Value: 1},
			{Key: "_id", Value: 1},
		}
	}
}
