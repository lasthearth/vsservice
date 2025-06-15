package memberdto

import (
	"github.com/lasthearth/vsservice/internal/settlement/model"
)

type Member struct {
	UserId string `bson:"user_id"`
}

func (m *Member) ToModel() *model.Member {
	return &model.Member{
		UserId: m.UserId,
	}
}

func FromModel(model *model.Member) *Member {
	return &Member{
		UserId: model.UserId,
	}
}
