package event

// Type defines specific event categories
type Type string

const (
	PlayerCount Type = "player_count"
	PlayerList  Type = "player_list"
	WorldTime   Type = "world_time"
)
