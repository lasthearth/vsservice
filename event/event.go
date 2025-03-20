package event

type Event struct {
	Id string `json:"id"`
}

type PlayerCountEvent struct {
	Event
	Count int `json:"count"`
	Max   int `json:"max"`
}

type WorldTimeEvent struct {
	Event
	FormattedTime string `json:"time"`
}

type PlayerListEvent struct {
	Event
	Players []Player `json:"players"`
}

type Player struct {
	Event
	Name       string `json:"name"`
	TimePlayed int64  `json:"time_played"`
}
