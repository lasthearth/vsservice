package model

import (
	"time"
)

type SettlementType string

const (
	SettlementTypeCamp      SettlementType = "camp"
	SettlementTypeVillage   SettlementType = "village"
	SettlementTypeCity      SettlementType = "city"
	SettlementTypeProvince  SettlementType = "province"
	SettlementTypeOrden     SettlementType = "orden"
	SettlementTypeGuild     SettlementType = "guild"
	SettlementTypeGuildLvl2 SettlementType = "guild_lvl2"
)

// Settlement represents a settlement in the game
type Settlement struct {
	Id          string
	Name        string
	Type        SettlementType
	Leader      Member
	Members     []Member
	Coordinates Vector2
	Attachments []Attachment

	UpdatedAt time.Time
	CreatedAt time.Time
}
