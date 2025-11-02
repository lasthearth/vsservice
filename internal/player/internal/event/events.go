package event

type PlayerTryJoinReqEvent struct {
	PlayerName string `json:"player_name"`
	PlayerUid  string `json:"player_uid"`
}

type PlayerTryJoinRespEvent struct {
	Status string `json:"status"`
}

type PlayerJoinEvent struct {
	Stats              Stats `json:"stats"`
	OnlinePlayersCount int   `json:"online_players_count"`
}

type Stats struct {
	Id            int     `json:"id"`
	DeathCount    int     `json:"death_count"`
	Deaths        []Death `json:"deaths"`
	HoursPlayed   float32 `json:"hours_played"`
	Name          string  `json:"name"`
	PlayersKilled int     `json:"players_killed"`
}

type Death struct {
	Cause      string `json:"cause"`
	EntityName string `json:"entity_name"`
}

type PlayerLeaveEvent struct {
	Stats              Stats `json:"stats"`
	OnlinePlayersCount int   `json:"online_players_count"`
}
