package model

import (
	"errors"
	"testing"
)

// TestMembershipsMayJoin pins the two membership rules a player answers to: one group per kind, and
// a guild owner who stays out of settlements.
func TestMembershipsMayJoin(t *testing.T) {
	cases := []struct {
		name    string
		held    Memberships
		kind    SettlementKind
		wantErr error
	}{
		{"nothing held, settlement", Memberships{}, KindSettlement, nil},
		{"nothing held, guild", Memberships{}, KindGuild, nil},
		// The point of the feature: both kinds at the same time, in either order.
		{"settlement member joins a guild", Memberships{InSettlement: true}, KindGuild, nil},
		{"guild member joins a settlement", Memberships{InGuild: true}, KindSettlement, nil},
		{"second settlement", Memberships{InSettlement: true}, KindSettlement, ErrAlreadyInSettlement},
		{"second guild", Memberships{InGuild: true}, KindGuild, ErrAlreadyInGuild},
		// Only ownership binds: a guild member may live in a settlement, its owner may not.
		{"guild owner joins a settlement", Memberships{InGuild: true, OwnsGuild: true}, KindSettlement, ErrGuildOwnerInSettlement},
		{"guild owner joins a second guild", Memberships{InGuild: true, OwnsGuild: true}, KindGuild, ErrAlreadyInGuild},
		{"both held, settlement", Memberships{InSettlement: true, InGuild: true}, KindSettlement, ErrAlreadyInSettlement},
		{"both held, guild", Memberships{InSettlement: true, InGuild: true}, KindGuild, ErrAlreadyInGuild},
		{"kind that selects both", Memberships{}, KindAny, ErrUnknownKind},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.held.MayJoin(tc.kind); !errors.Is(err, tc.wantErr) {
				t.Fatalf("MayJoin(%q) = %v, want %v", tc.kind, err, tc.wantErr)
			}
		})
	}
}

// TestMembershipsMayOwnGuild pins the rule for founding a guild and taking its ownership. A guild
// member must pass: transfer and grant both name a member of that guild.
func TestMembershipsMayOwnGuild(t *testing.T) {
	cases := []struct {
		name    string
		held    Memberships
		wantErr error
	}{
		{"nothing held", Memberships{}, nil},
		{"member of the guild", Memberships{InGuild: true}, nil},
		{"owner already", Memberships{InGuild: true, OwnsGuild: true}, nil},
		{"settlement member", Memberships{InSettlement: true}, ErrGuildOwnerInSettlement},
		{"settlement owner", Memberships{InSettlement: true}, ErrGuildOwnerInSettlement},
		{"both, owning the guild", Memberships{InSettlement: true, InGuild: true, OwnsGuild: true}, ErrGuildOwnerInSettlement},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.held.MayOwnGuild(); !errors.Is(err, tc.wantErr) {
				t.Fatalf("MayOwnGuild() = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// TestSettlementKind pins the derivation the stored kind and both partial unique indexes rest on:
// the guild tier is the only guild, and every other tier — including one added later and an unknown
// one — is a settlement.
func TestSettlementKind(t *testing.T) {
	settlements := []SettlementType{
		SettlementTypeCamp, SettlementTypeVillage, SettlementTypeTownship,
		SettlementTypeCity, SettlementTypeProvince, SettlementType("khutor"), SettlementType(""),
	}
	for _, typ := range settlements {
		if got := typ.Kind(); got != KindSettlement {
			t.Errorf("%q.Kind() = %q, want %q", typ, got, KindSettlement)
		}
		if typ.IsGuild() {
			t.Errorf("%q.IsGuild() = true, want false", typ)
		}
	}

	if got := SettlementTypeGuild.Kind(); got != KindGuild {
		t.Errorf("guild.Kind() = %q, want %q", got, KindGuild)
	}
	if !SettlementTypeGuild.IsGuild() {
		t.Error("guild.IsGuild() = false, want true")
	}

	set := &Settlement{Type: SettlementTypeGuild}
	if !set.IsGuild() || set.Kind() != KindGuild {
		t.Errorf("guild document: IsGuild = %v, Kind = %q", set.IsGuild(), set.Kind())
	}
	if got := (&Settlement{Type: SettlementTypeCity}).Kind(); got != KindSettlement {
		t.Errorf("city document kind = %q, want %q", got, KindSettlement)
	}
	if got := (&SettlementVerification{Type: SettlementTypeGuild}).Kind(); got != KindGuild {
		t.Errorf("guild request kind = %q, want %q", got, KindGuild)
	}
}

// TestLvlUpKeepsAGuild pins the single tier: a guild submitted again is re-verified, not raised.
func TestLvlUpKeepsAGuild(t *testing.T) {
	req := SettlementVerification{Type: SettlementTypeGuild}
	for range 3 {
		req.LvlUp()
		if req.Type != SettlementTypeGuild {
			t.Fatalf("type = %q after LvlUp, want it to stay guild", req.Type)
		}
	}
}
