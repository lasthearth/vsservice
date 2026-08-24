package model

// JoinRequest represents a player's request to join a settlement.
type JoinRequest struct {
	Id           string
	UserId       string
	SettlementId string
}
