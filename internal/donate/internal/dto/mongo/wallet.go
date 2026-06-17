package dto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type Wallet struct {
	mongox.Model `bson:",inline"`
	PlayerID     string `bson:"player_id"`
	PlayerName   string `bson:"player_name"`
	Coins        int64  `bson:"coins"`
}

func (w Wallet) Id() bson.ObjectID { return w.Model.Id }
