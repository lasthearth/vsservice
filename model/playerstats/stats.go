package playerstats

type Stats struct {
	DeathCount    int     `json:"death_count"`
	Deaths        []Death `json:"deaths"`
	HoursPlayed   float32 `json:"hours_played"`
	ID            int64   `json:"id"`
	LastOnline    int64   `json:"last_online"`
	Name          string  `json:"name"`
	PlayersKilled int     `json:"players_killed"`
}
