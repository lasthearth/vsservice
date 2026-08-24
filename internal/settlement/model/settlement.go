package model

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/samber/lo"
)

type SettlementType string

const (
	// Лагерь
	SettlementTypeCamp SettlementType = "camp"
	// Деревня
	SettlementTypeVillage SettlementType = "village"
	// Поселок
	SettlementTypeTownship SettlementType = "township"
	// Город
	SettlementTypeCity SettlementType = "city"
	// Региональная провинция
	SettlementTypeProvince SettlementType = "province"
	// SettlementTypeGuild     SettlementType = "guild"
	// SettlementTypeGuildLvl2 SettlementType = "guild_lvl2"
)

const (
	maxRolesPerSettlement = 20
	maxRoleNameLen        = 64
	maxContactInfoLen     = 512
)

var (
	ErrRolesDisabled   = errors.New("roles are disabled for this settlement")
	ErrRoleLimit       = errors.New("role limit reached")
	ErrRoleNotFound    = errors.New("role not found")
	ErrRoleNameInvalid = errors.New("role name is empty, too long or contains forbidden characters")
	ErrMemberNotFound  = errors.New("member not found")
	ErrLastOwner       = errors.New("cannot remove the last owner")
	ErrAlreadyOwner    = errors.New("member is already an owner")
	ErrNotOwnerRole    = errors.New("owner role cannot be assigned through roles")
	ErrContactInvalid  = errors.New("contact info too long or contains forbidden characters")
	ErrTargetNotMember = errors.New("target user is not a member of this settlement")
)

// forbiddenText rejects control characters (except plain space) and the angle
// brackets that enable the most common HTML/script injection. Escaping is the
// consumer's job; the server only refuses obviously hostile input.
func forbiddenText(s string) bool {
	if strings.ContainsAny(s, "<>") {
		return true
	}
	for _, r := range s {
		if r != ' ' && unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func (s *Settlement) SetDiplomacy(diplomacy string) error {
	if diplomacy == "" {
		return errors.New("diplomacy cannot be empty")
	}
	r, size := utf8.DecodeRuneInString(diplomacy)
	s.Diplomacy = string(unicode.ToUpper(r)) + diplomacy[size:]
	return nil
}

func (s *Settlement) SetProfile(name, description string, attachments []Attachment) {
	s.Name = name
	s.Description = description
	s.Attachments = attachments
}

// Settlement represents a settlement in the game
type Settlement struct {
	Id          string
	Name        string
	Type        SettlementType
	Description string
	// Leader is retained only to populate the deprecated proto field; it is
	// derived from the first owner in Members and never the source of truth.
	Leader        Member
	Members       []Member
	Coordinates   Vector2
	Diplomacy     string
	Attachments   []Attachment
	TagIds        []string
	ImperialFavor int64
	Roles         []Role
	RolesEnabled  bool
	ContactInfo   string

	UpdatedAt time.Time
	CreatedAt time.Time
}

func (s *Settlement) AddFavor(amount int64) {
	s.ImperialFavor += amount
}

func (s *Settlement) DeductFavor(amount int64) error {
	if s.ImperialFavor < amount {
		return errors.New("insufficient imperial favor")
	}
	s.ImperialFavor -= amount
	return nil
}

// member returns a pointer to the member with the given id, or nil.
func (s *Settlement) member(userId string) *Member {
	for i := range s.Members {
		if s.Members[i].UserId == userId {
			return &s.Members[i]
		}
	}
	return nil
}

func (s *Settlement) ownerCount() int {
	return lo.CountBy(s.Members, func(m Member) bool { return m.IsOwner() })
}

// DeriveLeader sets the deprecated Leader field to the first owner member, for
// proto backward compatibility. Safe to call on any loaded settlement.
func (s *Settlement) DeriveLeader() {
	for _, m := range s.Members {
		if m.IsOwner() {
			s.Leader = m
			return
		}
	}
}

// HasPermission reports whether userId may perform an action requiring perm.
// Owners always pass. Custom-role permissions are ignored while roles are
// disabled.
func (s *Settlement) HasPermission(userId string, perm Permission) bool {
	m := s.member(userId)
	if m == nil {
		return false
	}
	if m.IsOwner() {
		return true
	}
	if !s.RolesEnabled {
		return false
	}
	for _, r := range s.Roles {
		if !m.HasRole(r.Id) {
			continue
		}
		if lo.Contains(r.Permissions, perm) {
			return true
		}
	}
	return false
}

func (s *Settlement) IsOwner(userId string) bool {
	m := s.member(userId)
	return m != nil && m.IsOwner()
}

// AddMember appends a plain member. Caller must ensure global uniqueness.
func (s *Settlement) AddMember(userId string) {
	s.Members = append(s.Members, Member{UserId: userId, RoleIds: []string{}})
}

// RemoveMember drops a member. Refuses to remove the last owner.
func (s *Settlement) RemoveMember(userId string) error {
	m := s.member(userId)
	if m == nil {
		return ErrMemberNotFound
	}
	if m.IsOwner() && s.ownerCount() <= 1 {
		return ErrLastOwner
	}
	s.Members = lo.Reject(s.Members, func(x Member, _ int) bool { return x.UserId == userId })
	return nil
}

// TransferOwnership moves the owner role from caller to a target member without
// changing the owner count. Both must be members; target must not already own.
func (s *Settlement) TransferOwnership(fromUserId, toUserId string) error {
	from := s.member(fromUserId)
	if from == nil || !from.IsOwner() {
		return ErrNotOwnerRole
	}
	to := s.member(toUserId)
	if to == nil {
		return ErrTargetNotMember
	}
	if to.IsOwner() {
		return ErrAlreadyOwner
	}
	from.RoleIds = lo.Reject(from.RoleIds, func(x string, _ int) bool { return x == OwnerRoleId })
	to.RoleIds = append(to.RoleIds, OwnerRoleId)
	return nil
}

// GrantOwner adds the owner role to a member (admin operation).
func (s *Settlement) GrantOwner(userId string) error {
	m := s.member(userId)
	if m == nil {
		return ErrTargetNotMember
	}
	if m.IsOwner() {
		return ErrAlreadyOwner
	}
	m.RoleIds = append(m.RoleIds, OwnerRoleId)
	return nil
}

// RevokeOwner removes the owner role from a member (admin operation). Refuses to
// remove the last owner.
func (s *Settlement) RevokeOwner(userId string) error {
	m := s.member(userId)
	if m == nil {
		return ErrMemberNotFound
	}
	if !m.IsOwner() {
		return nil
	}
	if s.ownerCount() <= 1 {
		return ErrLastOwner
	}
	m.RoleIds = lo.Reject(m.RoleIds, func(x string, _ int) bool { return x == OwnerRoleId })
	return nil
}

func (s *Settlement) roleExists(roleId string) bool {
	return lo.ContainsBy(s.Roles, func(r Role) bool { return r.Id == roleId })
}

// CreateRole adds a custom role. roleId is supplied by the caller (e.g. a new
// object id). Rejects while roles are disabled and enforces the role limit.
func (s *Settlement) CreateRole(roleId, name string, perms []Permission) error {
	if !s.RolesEnabled {
		return ErrRolesDisabled
	}
	if len(s.Roles) >= maxRolesPerSettlement {
		return ErrRoleLimit
	}
	if err := validateRoleName(name); err != nil {
		return err
	}
	s.Roles = append(s.Roles, Role{
		Id:          roleId,
		Name:        name,
		Permissions: sanitizePerms(perms),
	})
	return nil
}

// UpdateRole replaces a custom role's name and permissions.
func (s *Settlement) UpdateRole(roleId, name string, perms []Permission) error {
	if !s.RolesEnabled {
		return ErrRolesDisabled
	}
	if err := validateRoleName(name); err != nil {
		return err
	}
	for i := range s.Roles {
		if s.Roles[i].Id == roleId {
			s.Roles[i].Name = name
			s.Roles[i].Permissions = sanitizePerms(perms)
			return nil
		}
	}
	return ErrRoleNotFound
}

// DeleteRole removes a custom role and unassigns it from every member.
func (s *Settlement) DeleteRole(roleId string) error {
	if !s.roleExists(roleId) {
		return ErrRoleNotFound
	}
	s.Roles = lo.Reject(s.Roles, func(r Role, _ int) bool { return r.Id == roleId })
	for i := range s.Members {
		s.Members[i].RoleIds = lo.Reject(s.Members[i].RoleIds, func(x string, _ int) bool { return x == roleId })
	}
	return nil
}

// AssignRole grants a custom role to a member. The owner role is not assignable
// here — use GrantOwner/TransferOwnership.
func (s *Settlement) AssignRole(userId, roleId string) error {
	if !s.RolesEnabled {
		return ErrRolesDisabled
	}
	if roleId == OwnerRoleId {
		return ErrNotOwnerRole
	}
	if !s.roleExists(roleId) {
		return ErrRoleNotFound
	}
	m := s.member(userId)
	if m == nil {
		return ErrMemberNotFound
	}
	if !m.HasRole(roleId) {
		m.RoleIds = append(m.RoleIds, roleId)
	}
	return nil
}

// UnassignRole removes a custom role from a member. The owner role is not
// removable here — use RevokeOwner.
func (s *Settlement) UnassignRole(userId, roleId string) error {
	if roleId == OwnerRoleId {
		return ErrNotOwnerRole
	}
	m := s.member(userId)
	if m == nil {
		return ErrMemberNotFound
	}
	m.RoleIds = lo.Reject(m.RoleIds, func(x string, _ int) bool { return x == roleId })
	return nil
}

// SetRolesEnabled toggles moderation of custom roles.
func (s *Settlement) SetRolesEnabled(enabled bool) {
	s.RolesEnabled = enabled
}

// SetContactInfo validates and stores the public contact string.
func (s *Settlement) SetContactInfo(contact string) error {
	if utf8.RuneCountInString(contact) > maxContactInfoLen || forbiddenText(contact) {
		return ErrContactInvalid
	}
	s.ContactInfo = contact
	return nil
}

func validateRoleName(name string) error {
	if name == "" || utf8.RuneCountInString(name) > maxRoleNameLen || forbiddenText(name) {
		return ErrRoleNameInvalid
	}
	return nil
}

func sanitizePerms(perms []Permission) []Permission {
	out := make([]Permission, 0, len(perms))
	for _, p := range perms {
		if p.IsValid() && !lo.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}
