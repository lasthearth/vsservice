package player

type List struct {
	Id      string   `json:"id"`
	Players []Player `json:"players"`
}

type Player struct {
	Id         string `json:"id"`
	Name       string `json:"name"`
	TimePlayed int64  `json:"time_played"`
}
