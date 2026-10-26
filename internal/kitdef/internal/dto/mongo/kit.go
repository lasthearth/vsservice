package dto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// KitItem mirrors model.KitItem for persistence. bson tags are the frozen
// cross-language schema shared with the game (C#) — do not rename.
type KitItem struct {
	GameCode     string `bson:"game_code"`
	Type         string `bson:"type"`
	Quantity     int32  `bson:"quantity"`
	AttrSnapshot string `bson:"attr_snapshot"`
	ImageURL     string `bson:"image_url"`
}

// Kit is the shared `kits` document. The game writes code/items/created_at
// (plus the category chosen at capture) Mongo-direct; vsservice writes title
// and category (and bumps updated_at). code carries a UNIQUE index. No
// omitempty on category: documents written by the game always carry it, and
// old documents decode to "" — which reads as the "kit" default.
type Kit struct {
	mongox.Model `bson:",inline"`
	Code         string    `bson:"code"`
	Title        string    `bson:"title"`
	Category     string    `bson:"category"`
	Items        []KitItem `bson:"items"`
}

// Id satisfies pagination.Identifiable.
func (k Kit) Id() bson.ObjectID { return k.Model.Id }
