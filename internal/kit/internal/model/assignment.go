package model

import (
	"errors"
	"time"
)

// KitAssignment represents the relationship linking a specific kit to a player,
// making the kit's contents available within the Vintage Story game environment
type KitAssignment struct {
	Id           string
	UserId       string
	UserGameName string
	KitName      string
	Status       AssignmentStatus
	AssignedAt   time.Time
	DeliveredAt  *time.Time
	ClaimedAt    *time.Time
	AssignedBy   string
}

func NewKitAssignment(userId, userGameName, kitName, assignedBy string) *KitAssignment {
	return &KitAssignment{
		UserId:       userId,
		UserGameName: userGameName,
		KitName:      kitName,
		Status:       AssignmentStatusPending,
		AssignedAt:   time.Now(),
		AssignedBy:   assignedBy,
	}
}

// Validate validates assignment data according to specification
func (ka *KitAssignment) Validate() error {
	if ka.UserId == "" {
		return errors.New("user ID cannot be empty")
	}

	if ka.UserGameName == "" {
		return errors.New("user game name cannot be empty")
	}

	if ka.KitName == "" {
		return errors.New("kit name cannot be empty")
	}

	if ka.AssignedBy == "" {
		return errors.New("assigned by cannot be empty")
	}

	if !ka.Status.IsValid() {
		return errors.New("invalid assignment status")
	}

	if ka.AssignedAt.After(time.Now()) {
		return errors.New("assigned at cannot be in the future")
	}

	if ka.DeliveredAt != nil && ka.AssignedAt.After(*ka.DeliveredAt) {
		return errors.New("delivery time cannot be before assignment time")
	}

	if ka.ClaimedAt != nil && ka.DeliveredAt != nil && ka.DeliveredAt.After(*ka.ClaimedAt) {
		return errors.New("claim time cannot be before delivery time")
	}

	return nil
}

// TransitionTo transitions assignment to a new state
func (ka *KitAssignment) TransitionTo(status AssignmentStatus) error {
	// Validate the new status first
	if !status.IsValid() {
		return errors.New("invalid assignment status")
	}

	// Check if the transition is valid
	if !ka.isValidTransition(status) {
		return errors.New("invalid status transition")
	}

	switch status {
	case AssignmentStatusDelivered:
		now := time.Now()
		ka.DeliveredAt = &now
	case AssignmentStatusClaimed:
		now := time.Now()
		ka.ClaimedAt = &now
	}

	ka.Status = status
	return nil
}

// isValidTransition checks if the transition to the new status is valid
func (ka *KitAssignment) isValidTransition(newStatus AssignmentStatus) bool {
	switch ka.Status {
	case AssignmentStatusPending:
		return newStatus == AssignmentStatusDelivered
	case AssignmentStatusDelivered:
		return newStatus == AssignmentStatusClaimed
	case AssignmentStatusClaimed:
		// No further transitions allowed after claimed
		return false
	default:
		return false
	}
}

// IsDelivered checks if assignment has been delivered
func (ka *KitAssignment) IsDelivered() bool {
	return ka.Status == AssignmentStatusDelivered || ka.Status == AssignmentStatusClaimed
}

// IsClaimed checks if assignment has been claimed
func (ka *KitAssignment) IsClaimed() bool {
	return ka.Status == AssignmentStatusClaimed
}
