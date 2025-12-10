package service

const (
	module                 = "kit-event"
	KitGrantedEventSubject = "kit.granted"
	KitClaimedEventSubject = "kit.claimed"
	StreamName             = "reward-events"
)

type KitGrantedEvent struct {
	AssignmentID string `json:"assignment_id"`
	KitName      string `json:"kit_name"`
	UserGameName string `json:"user_game_name"`
	UserID       string `json:"user_id"`
}

type KitClaimedEvent struct {
	AssignmentID string `json:"assignment_id"`
}
