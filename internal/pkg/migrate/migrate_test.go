package migrate

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Self-check for the pure transformation helpers used by the settlement
// migration. Run: go test ./internal/pkg/migrate
func TestEnsureOwnerAndNormalize(t *testing.T) {
	// Leader not yet in members: prepended with owner role.
	members := normalizeMembers(bson.A{bson.M{"user_id": "b"}})
	members = ensureOwner(members, "a")
	if len(members) != 2 {
		t.Fatalf("want 2 members, got %d", len(members))
	}
	first := members[0].(bson.M)
	if first["user_id"] != "a" || !containsStr(first["role_ids"], "owner") {
		t.Fatalf("leader a must be prepended with owner role, got %v", first)
	}

	// Leader already a member: owner role added, not duplicated.
	m2 := normalizeMembers(bson.A{bson.M{"user_id": "a", "role_ids": bson.A{"recruiter"}}})
	m2 = ensureOwner(m2, "a")
	if len(m2) != 1 {
		t.Fatalf("want 1 member, got %d", len(m2))
	}
	roles := toA(m2[0].(bson.M)["role_ids"])
	if len(roles) != 2 || !containsStr(roles, "owner") || !containsStr(roles, "recruiter") {
		t.Fatalf("want [recruiter owner], got %v", roles)
	}

	// Idempotent: a second ensureOwner does not add a duplicate owner.
	m2 = ensureOwner(m2, "a")
	if got := toA(m2[0].(bson.M)["role_ids"]); len(got) != 2 {
		t.Fatalf("owner role must not be duplicated, got %v", got)
	}

	// normalizeMembers on a nil/absent field yields an empty array, not nil.
	if got := normalizeMembers(nil); got == nil || len(got) != 0 {
		t.Fatalf("want empty bson.A, got %v", got)
	}
}
