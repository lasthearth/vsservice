package event

type PlayerTryJoinReqEvent struct {
	PlayerName string `json:"player_name"`
	PlayerUid  string `json:"player_uid"`
}

type PlayerTryJoinRespEvent struct {
	Status string `json:"status"`
}

type PlayerJoinEvent struct {
	Stats struct {
		Id            string  `json:"id"`
		DeathCount    int     `json:"death_count"`
		Deaths        []death `json:"deaths"`
		HoursPlayed   int     `json:"hours_played"`
		Name          string  `json:"name"`
		PlayersKilled int     `json:"players_killed"`
	} `json:"stats"`
	OnlinePlayersCount int `json:"online_players_count"`
}

type death struct {
	Cause      string `json:"cause"`
	EntityName string `json:"entity_name"`
}

type PlayerLeaveEvent struct {
	Status string `json:"status"`
}
