package migrate

import (
	"slices"
	"testing"
)

// Self-check for the pure transformation helpers used by the settlement
// migration. Run: go test ./internal/pkg/migrate
func TestFoldMembers(t *testing.T) {
	// Leader not yet in members: prepended with owner role, no member lost.
	s := &oldSettlement{
		Leader:  &oldMember{UserID: "a"},
		Members: []oldMember{{UserID: "b"}},
	}
	got := foldMembers(s)
	if len(got) != 2 {
		t.Fatalf("want 2 members, got %d", len(got))
	}
	if got[0].UserID != "a" || !slices.Contains(got[0].RoleIDs, ownerRoleID) {
		t.Fatalf("leader a must be prepended with owner role, got %+v", got[0])
	}

	// Leader already a member: owner role added, existing role kept, not duped.
	s = &oldSettlement{
		Leader:  &oldMember{UserID: "a"},
		Members: []oldMember{{UserID: "a", RoleIDs: []string{"recruiter"}}},
	}
	got = foldMembers(s)
	if len(got) != 1 {
		t.Fatalf("want 1 member, got %d", len(got))
	}
	if !slices.Contains(got[0].RoleIDs, ownerRoleID) || !slices.Contains(got[0].RoleIDs, "recruiter") {
		t.Fatalf("want [recruiter owner], got %v", got[0].RoleIDs)
	}

	// Idempotent: folding an already-folded settlement changes nothing.
	again := ensureOwner(got, "a")
	if len(again[0].RoleIDs) != 2 {
		t.Fatalf("owner role must not be duplicated, got %v", again[0].RoleIDs)
	}

	// Invariant: fold never drops a member.
	s = &oldSettlement{
		Leader:  &oldMember{UserID: "lead"},
		Members: []oldMember{{UserID: "x"}, {UserID: "y"}, {UserID: "z"}},
	}
	if got := foldMembers(s); len(got) != 4 {
		t.Fatalf("want 4 members (3 + leader), got %d", len(got))
	}

	// nil role_ids are normalised to empty, never left nil.
	s = &oldSettlement{Members: []oldMember{{UserID: "x"}}}
	if got := foldMembers(s); got[0].RoleIDs == nil {
		t.Fatalf("role_ids must be non-nil")
	}
}
