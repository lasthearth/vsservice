package httpdto

// Stats represents player stats from in game data.
type Stats struct {
	DeathCount  int     `json:"death_count"`
	Deaths      []Death `json:"deaths"`
	HoursPlayed float32 `json:"hours_played"`
	// Seed when I design vintage story api for some reason I call it id.
	//
	// But I think it's better to call it seed.
	Seed          int64  `json:"id"`
	LastOnline    int64  `json:"last_online"`
	Name          string `json:"name"`
	PlayersKilled int    `json:"players_killed"`
}
