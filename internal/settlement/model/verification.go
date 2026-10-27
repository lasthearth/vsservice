package model

import "time"

type SettlementStatus string

const (
	SettlementStatusPending        SettlementStatus = "pending"
	SettlementStatusApproved       SettlementStatus = "approved"
	SettlementStatusRejected       SettlementStatus = "rejected"
	SettlementStatusUpdateRejected SettlementStatus = "update-rejected"
)

type SettlementVerification struct {
	Id          string
	Name        string
	Type        SettlementType
	Leader      Member
	Coordinates Vector2
	Attachments []Attachment
	Diplomacy   string
	Description string

	Status          SettlementStatus
	RejectionReason string
	UpdatedAt       time.Time
	CreatedAt       time.Time
}

// RecordResubmission stamps the request with the moment it was filed again. The request is
// erased with the player, not with the settlement, so a settlement deleted and re-created is
// approved from a request written a second time; this last-write stamp is what tells the two
// lives of one id apart.
func (s *SettlementVerification) RecordResubmission(at time.Time) {
	s.UpdatedAt = at
}

// LvlUp moves the request to the next tier of its kind. A guild has one tier and stays where it
// is, so a further guild submission only re-verifies the profile.
func (s *SettlementVerification) LvlUp() {
	switch s.Type {
	case SettlementTypeCamp:
		s.Type = SettlementTypeVillage
	case SettlementTypeVillage:
		s.Type = SettlementTypeTownship
	case SettlementTypeTownship:
		s.Type = SettlementTypeCity
	case SettlementTypeCity:
		s.Type = SettlementTypeProvince
	case SettlementTypeProvince, SettlementTypeGuild:
		break
	}
}
