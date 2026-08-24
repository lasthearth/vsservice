package model

import (
	"errors"
	"testing"
)

// Runnable self-check for the multi-owner / role invariants. Run: go test ./internal/settlement/model
func TestSettlementRoleInvariants(t *testing.T) {
	newSet := func() *Settlement {
		return &Settlement{
			RolesEnabled: true,
			Members: []Member{
				{UserId: "a", RoleIds: []string{OwnerRoleId}},
				{UserId: "b", RoleIds: []string{}},
			},
		}
	}

	// Owner passes any permission; plain member fails.
	s := newSet()
	if !s.HasPermission("a", PermInviteMember) {
		t.Fatal("owner must pass any permission")
	}
	if s.HasPermission("b", PermInviteMember) {
		t.Fatal("plain member must fail without a role")
	}

	// Custom role grants a permission; disabling roles suppresses it.
	if err := s.CreateRole("r1", "Recruiter", []Permission{PermInviteMember}); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if err := s.AssignRole("b", "r1"); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	if !s.HasPermission("b", PermInviteMember) {
		t.Fatal("role should grant invite permission")
	}
	s.SetRolesEnabled(false)
	if s.HasPermission("b", PermInviteMember) {
		t.Fatal("disabled roles must not apply")
	}
	if s.HasPermission("a", PermInviteMember) == false {
		t.Fatal("owner must still pass while roles disabled")
	}
	s.SetRolesEnabled(true)

	// owner role is not assignable/removable through the role methods.
	if err := s.AssignRole("b", OwnerRoleId); !errors.Is(err, ErrNotOwnerRole) {
		t.Fatalf("expected ErrNotOwnerRole, got %v", err)
	}

	// Last owner cannot leave or be revoked.
	if err := s.RemoveMember("a"); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("expected ErrLastOwner on removing sole owner, got %v", err)
	}
	if err := s.RevokeOwner("a"); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("expected ErrLastOwner on revoking sole owner, got %v", err)
	}

	// Transfer keeps owner count at 1 and swaps the holder.
	if err := s.TransferOwnership("a", "b"); err != nil {
		t.Fatalf("TransferOwnership: %v", err)
	}
	if s.ownerCount() != 1 {
		t.Fatalf("owner count must stay 1, got %d", s.ownerCount())
	}
	if !s.IsOwner("b") || s.IsOwner("a") {
		t.Fatal("ownership must move from a to b")
	}

	// Admin GrantOwner adds a second owner; now the first can leave.
	if err := s.GrantOwner("a"); err != nil {
		t.Fatalf("GrantOwner: %v", err)
	}
	if s.ownerCount() != 2 {
		t.Fatalf("owner count must be 2, got %d", s.ownerCount())
	}
	if err := s.RemoveMember("a"); err != nil {
		t.Fatalf("removing a non-last owner must succeed, got %v", err)
	}

	// Contact info rejects angle brackets and overlong input.
	if err := s.SetContactInfo("discord: <script>"); !errors.Is(err, ErrContactInvalid) {
		t.Fatalf("expected ErrContactInvalid for angle brackets, got %v", err)
	}
	if err := s.SetContactInfo("discord: hearth#1"); err != nil {
		t.Fatalf("valid contact info rejected: %v", err)
	}

	// DeleteRole unassigns from every member.
	s2 := newSet()
	_ = s2.CreateRole("r2", "Guard", nil)
	_ = s2.AssignRole("b", "r2")
	if err := s2.DeleteRole("r2"); err != nil {
		t.Fatalf("DeleteRole: %v", err)
	}
	if s2.member("b").HasRole("r2") {
		t.Fatal("deleted role must be unassigned from members")
	}
}
