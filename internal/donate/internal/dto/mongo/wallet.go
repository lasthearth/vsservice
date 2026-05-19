package dto

import "github.com/lasthearth/vsservice/internal/pkg/mongox"

type Wallet struct {
	mongox.Model `bson:",inline"`
	PlayerID     string `bson:"player_id"`
	PlayerName   string `bson:"player_name"`
	Coins        int64  `bson:"coins"`
}
