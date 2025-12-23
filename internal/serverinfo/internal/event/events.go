package event

type WorldTimeEvent struct {
	Time string `json:"time"`
}

type TotalOnlineEvent struct {
	Count int `json:"count"`
	Max   int `json:"max"`
}
