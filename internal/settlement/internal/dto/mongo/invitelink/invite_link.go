package invitelinkdto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
)

// InviteLink is the `settlement_invite_links` document. code carries a unique
// index; settlement_id is indexed for the leader's list.
type InviteLink struct {
	mongox.Model `bson:",inline"`
	SettlementId string     `bson:"settlement_id"`
	Code         string     `bson:"code"`
	CreatedBy    string     `bson:"created_by"`
	MaxUses      int32      `bson:"max_uses"`
	Uses         int32      `bson:"uses"`
	ExpiresAt    *time.Time `bson:"expires_at,omitempty"`
	RevokedAt    *time.Time `bson:"revoked_at,omitempty"`
}
