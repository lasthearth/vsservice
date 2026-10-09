package dto

import "time"

// Appearance is a document of the appearances collection, one per player.
type Appearance struct {
	UserId       string    `bson:"user_id"`
	BannerId     string    `bson:"banner_id"`
	BannerEffect string    `bson:"banner_effect"`
	FrameId      string    `bson:"frame_id"`
	FrameEffect  string    `bson:"frame_effect"`
	TitleKey     string    `bson:"title_key"`
	UpdatedAt    time.Time `bson:"updated_at"`
}
