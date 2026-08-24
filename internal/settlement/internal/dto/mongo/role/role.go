package roledto

import "github.com/lasthearth/vsservice/internal/settlement/model"

type Role struct {
	Id          string   `bson:"id"`
	Name        string   `bson:"name"`
	Permissions []string `bson:"permissions"`
}

func (r *Role) ToModel() *model.Role {
	perms := make([]model.Permission, len(r.Permissions))
	for i, p := range r.Permissions {
		perms[i] = model.Permission(p)
	}
	return &model.Role{
		Id:          r.Id,
		Name:        r.Name,
		Permissions: perms,
	}
}

func FromModel(m *model.Role) *Role {
	perms := make([]string, len(m.Permissions))
	for i, p := range m.Permissions {
		perms[i] = string(p)
	}
	return &Role{
		Id:          m.Id,
		Name:        m.Name,
		Permissions: perms,
	}
}
