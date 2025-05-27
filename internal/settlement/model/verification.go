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

	Status          SettlementStatus
	RejectionReason string
	UpdatedAt       time.Time
	CreatedAt       time.Time
}
