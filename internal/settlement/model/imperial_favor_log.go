package model

import "time"

type ImperialFavorLog struct {
	Id           string
	SettlementId string
	AdminId      string
	Amount       int64
	Reason       string
	CreatedAt    time.Time
}
