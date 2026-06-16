package model

import (
	"errors"
	"time"
	"unicode"
	"unicode/utf8"
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

func (s *Settlement) SetDiplomacy(diplomacy string) error {
	if diplomacy == "" {
		return errors.New("diplomacy cannot be empty")
	}
	r, size := utf8.DecodeRuneInString(diplomacy)
	s.Diplomacy = string(unicode.ToUpper(r)) + diplomacy[size:]
	return nil
}

func (s *Settlement) SetProfile(name, description string, attachments []Attachment) {
	s.Name = name
	s.Description = description
	s.Attachments = attachments
}

// Settlement represents a settlement in the game
type Settlement struct {
	Id            string
	Name          string
	Type          SettlementType
	Description   string
	Leader        Member
	Members       []Member
	Coordinates   Vector2
	Diplomacy     string
	Attachments   []Attachment
	TagIds        []string
	ImperialFavor int64

	UpdatedAt time.Time
	CreatedAt time.Time
}

func (s *Settlement) AddFavor(amount int64) {
	s.ImperialFavor += amount
}

func (s *Settlement) DeductFavor(amount int64) error {
	if s.ImperialFavor < amount {
		return errors.New("insufficient imperial favor")
	}
	s.ImperialFavor -= amount
	return nil
}
