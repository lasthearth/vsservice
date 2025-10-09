package service

import "github.com/lasthearth/vsservice/internal/settlement/model"

type SettlementOpts struct {
	Name        string
	Type        model.SettlementType
	Leader      model.Member
	Coordinates model.Vector2
	Attachments []model.Attachment
	Diplomacy   string
	Description string
}

type UpdateSettlementOpts struct {
	ID          string
	Name        string
	Type        model.SettlementType
	Leader      model.Member
	Coordinates model.Vector2
	Attachments []model.Attachment
	Diplomacy   string
	Description string
}
