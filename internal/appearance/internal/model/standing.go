package model

// TopRank is the last place that still counts as the top of the table.
const TopRank = 10

// SettlementRole is the player's place in a settlement.
type SettlementRole string

const (
	SettlementNone     SettlementRole = ""
	SettlementResident SettlementRole = "resident"
	SettlementLeader   SettlementRole = "leader"
)

// Standing is what unlocks appearance items: the player's totals on the
// server, their places in the table, their settlement, what they did on the
// site and what they bought.
type Standing struct {
	Hours  float64
	Kills  int
	Deaths int
	// HoursRank and KillsRank are 1-based places; 0 means not in the table.
	HoursRank      int
	KillsRank      int
	SettlementRole SettlementRole
	// HungerGamesWins sums the wins over every Hunger Games season.
	HungerGamesWins int
	// Referrals counts the players who joined with this player's code.
	Referrals int
	// Events counts the events the player signed up for.
	Events int
	// Days on the server, counted from the player's application.
	Days int
	// Purchased holds the ids of items bought with shards.
	Purchased map[string]bool
}

// inTop reports whether the player is in the top by hours or by kills.
func (s Standing) inTop() bool {
	in := func(rank int) bool { return rank > 0 && rank <= TopRank }
	return in(s.HoursRank) || in(s.KillsRank)
}

// survives reports whether the player has played at least hours and died at
// most once per ten hours.
func (s Standing) survives(hours int) bool {
	return s.Hours >= float64(hours) && float64(s.Deaths)*10 <= s.Hours
}

// Owns reports whether the player bought the item.
func (s Standing) Owns(itemID string) bool {
	return s.Purchased[itemID]
}
