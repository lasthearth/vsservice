package dto

import "github.com/lasthearth/vsservice/internal/pkg/mongox"

type Tag struct {
	mongox.Model `bson:",inline"`
	Name         string `bson:"name"`
	Color        Color  `bson:"color,omitempty"`
	Description  string `bson:"description,omitempty"`
	IsActive     bool   `bson:"is_active"`
}

type Color struct {
	Red   float32 `bson:"red"`
	Green float32 `bson:"green"`
	Blue  float32 `bson:"blue"`
	Alpha float32 `bson:"alpha"`
}
