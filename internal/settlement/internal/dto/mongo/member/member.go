package memberdto

import (
	"github.com/lasthearth/vsservice/internal/settlement/model"
)

type Member struct {
	UserId  string   `bson:"user_id"`
	RoleIds []string `bson:"role_ids"`
}

func (m *Member) ToModel() *model.Member {
	return &model.Member{
		UserId:  m.UserId,
		RoleIds: m.RoleIds,
	}
}

func FromModel(model *model.Member) *Member {
	return &Member{
		UserId:  model.UserId,
		RoleIds: model.RoleIds,
	}
}
