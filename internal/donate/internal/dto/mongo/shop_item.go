package dto

import "github.com/lasthearth/vsservice/internal/pkg/mongox"

type ShopItem struct {
	mongox.Model `bson:",inline"`
	Name         string `bson:"name"`
	Description  string `bson:"description"`
	ImageURL     string `bson:"image_url"`
	Price        int64  `bson:"price"`
	IsAvailable  bool   `bson:"is_available"`
}
