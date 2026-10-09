package repository

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/lasthearth/vsservice/internal/appearance/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// ownerRoleID marks the settlement's leaders among its members
// (settlement/model.OwnerRoleId).
const ownerRoleID = "owner"

// totals is one player's summed stats, grouped like the leaderboard does.
type totals struct {
	Name   string  `bson:"_id"`
	Hours  float64 `bson:"hours"`
	Kills  int     `bson:"kills"`
	Deaths int     `bson:"deaths"`
}

// player is what the players collection tells about the player.
type player struct {
	Name      string    `bson:"user_game_name"`
	CreatedAt time.Time `bson:"created_at"`
}

// Standing gathers everything that unlocks items: game totals and places,
// settlement, Hunger Games wins, invited players, event sign-ups, days on the
// server and purchases. A player with no stats yet stands at zero there.
func (s *Standings) Standing(ctx context.Context, userID string) (model.Standing, error) {
	log := s.logger.WithMethod("Standing")
	fail := func(what string, err error) (model.Standing, error) {
		log.Error("failed to read "+what, zap.Error(err))
		return model.Standing{}, err
	}

	role, err := s.settlementRole(ctx, userID)
	if err != nil {
		return fail("settlement", err)
	}
	wins, err := s.hungerGamesWins(ctx, userID)
	if err != nil {
		return fail("hunger games stats", err)
	}
	referrals, err := s.referrals.CountDocuments(ctx, bson.M{"referrer_player_id": userID})
	if err != nil {
		return fail("referrals", err)
	}
	events, err := s.attendees.CountDocuments(ctx, bson.M{"user_id": userID})
	if err != nil {
		return fail("event sign-ups", err)
	}
	owned, err := s.purchased(ctx, userID)
	if err != nil {
		return fail("purchases", err)
	}
	who, err := s.player(ctx, userID)
	if err != nil {
		return fail("player", err)
	}

	days := 0
	if !who.CreatedAt.IsZero() {
		days = int(s.now().Sub(who.CreatedAt).Hours() / 24)
	}

	mine, hoursRank, killsRank := totals{}, 0, 0
	if who.Name != "" {
		all, err := s.allTotals(ctx)
		if err != nil {
			return fail("stats", err)
		}
		mine, hoursRank, killsRank = place(all, who.Name)
	}

	return model.Standing{
		Hours:           mine.Hours,
		Kills:           mine.Kills,
		Deaths:          mine.Deaths,
		HoursRank:       hoursRank,
		KillsRank:       killsRank,
		SettlementRole:  role,
		HungerGamesWins: wins,
		Referrals:       int(referrals),
		Events:          int(events),
		Days:            days,
		Purchased:       owned,
	}, nil
}

// place finds the player's totals and places. A place is one more than the
// number of players strictly ahead, so ties share a place — the way the site
// counts it. A player outside the table gets zero totals and no places.
func place(all []totals, name string) (mine totals, hoursRank, killsRank int) {
	found := false
	for _, t := range all {
		if t.Name == name {
			mine, found = t, true
			break
		}
	}
	if !found {
		return totals{}, 0, 0
	}

	hoursRank, killsRank = 1, 1
	for _, t := range all {
		if t.Hours > mine.Hours {
			hoursRank++
		}
		if t.Kills > mine.Kills {
			killsRank++
		}
	}
	return mine, hoursRank, killsRank
}

// player returns the player's in-game name and application date, or zero
// values if the player is unknown.
func (s *Standings) player(ctx context.Context, userID string) (player, error) {
	var doc player
	err := s.players.FindOne(
		ctx,
		bson.M{"user_id": userID},
		options.FindOne().SetProjection(bson.M{"user_game_name": 1, "created_at": 1}),
	).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return player{}, nil
	}
	return doc, err
}

// hungerGamesWins sums the player's wins over every season: the completed
// seasons archived in hg_season_results plus the current one in hg_player_stats.
// ResetSeason archives the standings and then wipes hg_player_stats, so the
// current collection alone would drop every past win on each reset.
func (s *Standings) hungerGamesWins(ctx context.Context, userID string) (int, error) {
	sumWins := func(coll *mongo.Collection) (int, error) {
		pipeline := bson.A{
			bson.D{{Key: "$match", Value: bson.M{"player_id": userID}}},
			bson.D{{Key: "$group", Value: bson.D{
				{Key: "_id", Value: nil},
				{Key: "wins", Value: bson.D{{Key: "$sum", Value: "$wins"}}},
			}}},
		}
		cursor, err := coll.Aggregate(ctx, pipeline)
		if err != nil {
			return 0, err
		}
		var out []struct {
			Wins int `bson:"wins"`
		}
		if err := cursor.All(ctx, &out); err != nil {
			return 0, err
		}
		if len(out) == 0 {
			return 0, nil
		}
		return out[0].Wins, nil
	}

	archived, err := sumWins(s.seasonResults)
	if err != nil {
		return 0, err
	}
	current, err := sumWins(s.hgStats)
	if err != nil {
		return 0, err
	}
	return archived + current, nil
}

// allTotals sums every player's stats by in-game name.
func (s *Standings) allTotals(ctx context.Context) ([]totals, error) {
	pipeline := bson.A{
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$user_game_name"},
			{Key: "hours", Value: bson.D{{Key: "$sum", Value: "$hours_played"}}},
			{Key: "kills", Value: bson.D{{Key: "$sum", Value: "$players_killed"}}},
			{Key: "deaths", Value: bson.D{{Key: "$sum", Value: "$death_count"}}},
		}}},
	}
	cursor, err := s.stats.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}

	var out []totals
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// settlementRole finds the settlement the player belongs to.
//
// Leadership is read from the member's owner role only, the way the
// settlement domain itself decides it (IsLeaderOfSettlement). The stored
// `leader` field is deliberately ignored: the settlement model keeps it only
// to populate a deprecated proto field, and it goes stale the moment
// ownership is transferred or the founder leaves. The unique index on
// members.user_id also guarantees one settlement per player, so this match
// never needs the $or the stale leader field would force.
func (s *Standings) settlementRole(ctx context.Context, userID string) (model.SettlementRole, error) {
	var doc struct {
		Members []struct {
			UserId  string   `bson:"user_id"`
			RoleIds []string `bson:"role_ids"`
		} `bson:"members"`
	}
	err := s.settlements.FindOne(
		ctx,
		bson.M{"members.user_id": userID},
		options.FindOne().SetProjection(bson.M{"members.user_id": 1, "members.role_ids": 1}),
	).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.SettlementNone, nil
	}
	if err != nil {
		return model.SettlementNone, err
	}

	for _, m := range doc.Members {
		if m.UserId != userID {
			continue
		}
		if slices.Contains(m.RoleIds, ownerRoleID) {
			return model.SettlementLeader, nil
		}
		return model.SettlementResident, nil
	}
	return model.SettlementResident, nil
}
