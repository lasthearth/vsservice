package event

type PlayerTryJoinReqEvent struct {
	PlayerName string `json:"player_name"`
	PlayerUid  string `json:"player_uid"`
}

type PlayerTryJoinRespEvent struct {
	Status string `json:"status"`
}

type PlayerJoinEvent struct {
	PlayerName string `json:"player_name"`
	PlayerUid  string `json:"player_uid"`
}

type PlayerLeaveEvent struct {
	PlayerName string `json:"player_name"`
	PlayerUid  string `json:"player_uid"`
}
