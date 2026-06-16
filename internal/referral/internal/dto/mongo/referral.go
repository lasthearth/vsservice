package dto

import "github.com/lasthearth/vsservice/internal/pkg/mongox"

type ReferralCode struct {
	mongox.Model `bson:",inline"`
	PlayerID     string `bson:"player_id"`
	PlayerName   string `bson:"player_name"`
	Code         string `bson:"code"`
}

type ReferralEvent struct {
	mongox.Model     `bson:",inline"`
	ReferrerPlayerID string `bson:"referrer_player_id"`
	RefereePlayerID  string `bson:"referee_player_id"`
	CoinsAwarded     int64  `bson:"coins_awarded"`
}
