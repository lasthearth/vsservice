package dto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// MailClaim is the per-player state row for one (MailID, PlayerID) pair.
type MailClaim struct {
	mongox.Model `bson:",inline"`
	MailID       string     `bson:"mail_id"`
	PlayerID     string     `bson:"player_id"`
	State        string     `bson:"state"`
	ReadAt       *time.Time `bson:"read_at,omitempty"`
	ClaimedAt    *time.Time `bson:"claimed_at,omitempty"`
}

// Id satisfies pagination.Identifiable.
func (c MailClaim) Id() bson.ObjectID { return c.Model.Id }
