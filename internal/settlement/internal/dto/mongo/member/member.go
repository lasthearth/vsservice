package memberdto

import (
	"github.com/lasthearth/vsservice/internal/settlement/model"
)

type Member struct {
	UserID string `bson:"user_id"`
}

func (m *Member) ToModel() *model.Member {
	return &model.Member{
		UserID: m.UserID,
	}
}

func FromModel(model *model.Member) *Member {
	return &Member{
		UserID: model.UserID,
	}
}
