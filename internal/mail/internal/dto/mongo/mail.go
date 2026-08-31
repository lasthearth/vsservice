package dto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ItemAttachment mirrors model.ItemAttachment for persistence.
type ItemAttachment struct {
	GameCode     string `bson:"game_code"`
	Quantity     int32  `bson:"quantity"`
	AttrSnapshot string `bson:"attr_snapshot,omitempty"`
	Type         string `bson:"type,omitempty"`
}

// CoinsAttachment mirrors model.CoinsAttachment for persistence.
type CoinsAttachment struct {
	Amount int64 `bson:"amount"`
}

// Attachment is exactly one of Item or Coins.
type Attachment struct {
	Item  *ItemAttachment  `bson:"item,omitempty"`
	Coins *CoinsAttachment `bson:"coins,omitempty"`
}

// Mail is the immutable content document.
type Mail struct {
	mongox.Model   `bson:",inline"`
	Recipient      string       `bson:"recipient"`
	Sender         string       `bson:"sender"`
	Title          string       `bson:"title"`
	Body           string       `bson:"body"`
	Attachments    []Attachment `bson:"attachments,omitempty"`
	ExpiresAt      *time.Time   `bson:"expires_at,omitempty"`
	Revoked        bool         `bson:"revoked"`
	IdempotencyKey string       `bson:"idempotency_key,omitempty"`
}

// Id satisfies pagination.Identifiable.
func (m Mail) Id() bson.ObjectID { return m.Model.Id }
