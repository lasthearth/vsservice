package model

// AssignmentStatus represents the lifecycle status of a kit assignment
type AssignmentStatus string

const (
	// AssignmentStatusPending means assignment created but not yet delivered to game
	AssignmentStatusPending AssignmentStatus = "PENDING"

	// AssignmentStatusDelivered means assignment sent to game system
	AssignmentStatusDelivered AssignmentStatus = "DELIVERED"

	// AssignmentStatusClaimed means user has claimed the kit in game
	AssignmentStatusClaimed AssignmentStatus = "CLAIMED"
)

// IsValid checks if the assignment status is valid
func (as AssignmentStatus) IsValid() bool {
	switch as {
	case AssignmentStatusPending, AssignmentStatusDelivered, AssignmentStatusClaimed:
		return true
	default:
		return false
	}
}
