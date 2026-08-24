package model

// Permission is a capability a custom role can grant. The owner role holds all
// permissions implicitly and is never checked through this set.
type Permission string

const (
	PermInviteMember      Permission = "invite_member"
	PermReviewJoinRequest Permission = "review_join_request"
)

// OwnerRoleId is the built-in role id that marks a settlement leader. It is not
// stored in Settlement.Roles and cannot be created, edited or deleted.
const OwnerRoleId = "owner"

func (p Permission) IsValid() bool {
	switch p {
	case PermInviteMember, PermReviewJoinRequest:
		return true
	default:
		return false
	}
}

// Role is a custom, owner-defined role. Name is an RP title; Permissions is a
// small closed set of capabilities.
type Role struct {
	Id          string
	Name        string
	Permissions []Permission
}
