package model

import "time"

// LastSeenWindow narrows the roster to the players the game saw inside a
// window. Every window includes the shorter ones, so
// ONLINE ⊆ DAY ⊆ WEEK ⊆ MONTH, and INACTIVE is what is left outside MONTH.
type LastSeenWindow uint8

const (
	// LastSeenAny is every player, whatever the game recorded.
	LastSeenAny LastSeenWindow = iota
	LastSeenOnline
	LastSeenDay
	LastSeenWeek
	LastSeenMonth
	LastSeenInactive
)

// PlayerListSort is the order of the roster.
type PlayerListSort uint8

const (
	// PlayerSortLastSeen puts who is in game first, then the most recently
	// seen.
	PlayerSortLastSeen PlayerListSort = iota
	PlayerSortHours
	PlayerSortNickname
)

// PlayerListFilter is what the roster page asked for, already validated and
// clamped by the service.
type PlayerListFilter struct {
	Query    string
	LastSeen LastSeenWindow
	Sort     PlayerListSort
}

// PlayerListItem is one roster row. Hours and LastOnline are the same numbers
// the leaderboard shows, summed over the player's stats documents.
type PlayerListItem struct {
	UserId      string
	Nickname    string
	Avatar      *Avatar
	IsOnline    bool
	LastOnline  *time.Time
	HoursPlayed float64
	About       string
}

// LastSeenCounts is how many players each window holds for the same query.
type LastSeenCounts struct {
	All      int32
	Online   int32
	Day      int32
	Week     int32
	Month    int32
	Inactive int32
}

// PlayerListPage is one page of the roster with its totals.
type PlayerListPage struct {
	Players   []PlayerListItem
	TotalSize int32
	Page      int32
	PageSize  int32
	Counts    LastSeenCounts
}
