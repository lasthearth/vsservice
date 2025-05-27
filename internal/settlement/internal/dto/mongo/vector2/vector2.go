package vector2dto

import "github.com/lasthearth/vsservice/internal/settlement/model"

type Vector2 struct {
	X int `bson:"x"`
	Y int `bson:"y"`
}

func (v *Vector2) ToModel() *model.Vector2 {
	return &model.Vector2{
		X: v.X,
		Y: v.Y,
	}
}

func FromModel(model *model.Vector2) *Vector2 {
	return &Vector2{
		X: model.X,
		Y: model.Y,
	}
}
