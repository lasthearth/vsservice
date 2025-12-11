package mongo

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
)

// Assignment represents the KitAssignment model for MongoDB storage
type Assignment struct {
	mongox.Model
	UserId       string     `bson:"user_id"`
	KitName      string     `bson:"kit_name"`
	Status       string     `bson:"status"`
	UserGameName string     `bson:"user_game_name"`
	AssignedAt   time.Time  `bson:"assigned_at"`
	DeliveredAt  *time.Time `bson:"delivered_at,omitempty"`
	ClaimedAt    *time.Time `bson:"claimed_at,omitempty"`
	AssignedBy   string     `bson:"assigned_by"`
}
