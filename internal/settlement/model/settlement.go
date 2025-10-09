package model

import (
	"time"
)

type SettlementType string

const (
	// Лагерь
	SettlementTypeCamp SettlementType = "camp"
	// Деревня
	SettlementTypeVillage SettlementType = "village"
	// Поселок
	SettlementTypeTownship SettlementType = "township"
	// Город
	SettlementTypeCity SettlementType = "city"
	// Региональная провинция
	SettlementTypeProvince SettlementType = "province"
	// SettlementTypeGuild     SettlementType = "guild"
	// SettlementTypeGuildLvl2 SettlementType = "guild_lvl2"
)

// Settlement represents a settlement in the game
type Settlement struct {
	Id          string
	Name        string
	Type        SettlementType
	Description string
	Leader      Member
	Members     []Member
	Coordinates Vector2
	Diplomacy   string
	Attachments []Attachment

	UpdatedAt time.Time
	CreatedAt time.Time
}
