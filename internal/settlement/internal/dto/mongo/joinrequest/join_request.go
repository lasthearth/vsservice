package joinrequestdto

import (
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type JoinRequest struct {
	Id           bson.ObjectID `bson:"_id"`
	UserId       string        `bson:"user_id"`
	SettlementId string        `bson:"settlement_id"`
	// Kind copies the kind of the group applied to, so clearing a player's applications after a
	// join can leave the other kind alone: joining a settlement must not withdraw the player's
	// pending guild applications. It is not part of the model, which has no use for it.
	Kind string `bson:"kind"`
}

func (j *JoinRequest) ToModel() *model.JoinRequest {
	return &model.JoinRequest{
		Id:           j.Id.Hex(),
		UserId:       j.UserId,
		SettlementId: j.SettlementId,
	}
}
