package favorlogdto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type ImperialFavorLog struct {
	mongox.Model `bson:",inline"`
	SettlementId bson.ObjectID `bson:"settlement_id"`
	AdminId      string        `bson:"admin_id"`
	Amount       int64         `bson:"amount"`
	Reason       string        `bson:"reason"`
}

func (l ImperialFavorLog) Id() bson.ObjectID {
	return l.Model.Id
}
