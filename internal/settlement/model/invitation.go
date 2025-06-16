package model

// Invitation represents an invitation to join a settlement.
type Invitation struct {
	Id           string
	UserId       string
	SettlementId string
}
