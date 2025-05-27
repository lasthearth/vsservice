package model

import "time"

type SettlementType string

const (
	SettlementTypeVillage  SettlementType = "village"
	SettlementTypeCity     SettlementType = "city"
	SettlementTypeProvince SettlementType = "province"
)

// Settlement represents a settlement in the game
type Settlement struct {
	ID          string
	Name        string
	Type        SettlementType
	Leader      Member
	Members     []Member
	Coordinates Vector2
	Attachments []Attachment

	UpdatedAt time.Time
	CreatedAt time.Time
}
