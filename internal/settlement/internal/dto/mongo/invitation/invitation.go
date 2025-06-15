package invitationdto

import "go.mongodb.org/mongo-driver/bson/primitive"

type Invitation struct {
	Id           primitive.ObjectID `bson:"_id"`
	UserId       string             `bson:"user_id"`
	SettlementId string             `bson:"settlement_id"`
}
