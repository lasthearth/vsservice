package model

import "github.com/samber/lo"

type Member struct {
	UserId  string
	RoleIds []string
}

// IsOwner reports whether the member holds the built-in owner role.
func (m *Member) IsOwner() bool {
	return lo.Contains(m.RoleIds, OwnerRoleId)
}

// HasRole reports whether the member holds the given role id.
func (m *Member) HasRole(roleId string) bool {
	return lo.Contains(m.RoleIds, roleId)
}
