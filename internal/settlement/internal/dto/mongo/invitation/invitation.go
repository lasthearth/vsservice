package invitationdto

import (
	"go.mongodb.org/mongo-driver/v2/bson"
)

type Invitation struct {
	Id           bson.ObjectID `bson:"_id"`
	UserId       string        `bson:"user_id"`
	SettlementId string        `bson:"settlement_id"`
}
