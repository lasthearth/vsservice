package joinrequestdto

import (
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type JoinRequest struct {
	Id           bson.ObjectID `bson:"_id"`
	UserId       string        `bson:"user_id"`
	SettlementId string        `bson:"settlement_id"`
}

func (j *JoinRequest) ToModel() *model.JoinRequest {
	return &model.JoinRequest{
		Id:           j.Id.Hex(),
		UserId:       j.UserId,
		SettlementId: j.SettlementId,
	}
}
