package event

import "encoding/json"

// Event represents a message with type and payload
type Event struct {
	Type Type            `json:"event_type"`
	Data json.RawMessage `json:"data"`
}

type PlayerCountEvent struct {
	Count int `json:"count"`
}

type WorldTimeEvent struct {
	FormattedTime string `json:"time"`
}

type PlayerListEvent struct {
	Players []Player `json:"players"`
}

type Player struct {
	Name       string `json:"name"`
	TimePlayed int64  `json:"time_played"`
}
