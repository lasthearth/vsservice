package model

import "time"

// SeasonResult is the archived snapshot of a player's final standing
// in a completed season.
type SeasonResult struct {
	ID          string
	SeasonID    string
	PlayerID    string
	PlayerName  string
	Elo         int
	Wins        int
	Kills       int
	Rank        int
	RewardCoins int64
	CreatedAt   time.Time
}
