package model

import (
	"slices"
	"testing"
)

func TestNotifierBlockCode(t *testing.T) {
	cases := []struct {
		typ  SettlementType
		want string
		ok   bool
	}{
		{SettlementTypeCamp, "lhgui:notifier-camp", true},
		{SettlementTypeVillage, "lhgui:notifier-village", true},
		// The block variant is "town", not "township".
		{SettlementTypeTownship, "lhgui:notifier-town", true},
		{SettlementTypeCity, "lhgui:notifier-city", true},
		{SettlementTypeProvince, "lhgui:notifier-province", true},
		// Outside the automation: hamlet and imperial tiers have no settlement type.
		{SettlementType("khutor"), "", false},
		{SettlementType("hamlet"), "", false},
		{SettlementType("abandoned"), "", false},
		{SettlementType("normal"), "", false},
		{SettlementType("mine"), "", false},
		{SettlementType(""), "", false},
	}
	for _, tc := range cases {
		got, ok := tc.typ.NotifierBlockCode()
		if got != tc.want || ok != tc.ok {
			t.Errorf("%q.NotifierBlockCode() = (%q, %v), want (%q, %v)", tc.typ, got, ok, tc.want, tc.ok)
		}
	}
}

// Every settlement type the site can hold has a block, so a created or
// upgraded settlement is never left without a tier to hand out.
func TestEveryLevelUpTypeHasBlock(t *testing.T) {
	v := SettlementVerification{Type: SettlementTypeCamp}
	for range 6 {
		if _, ok := v.Type.NotifierBlockCode(); !ok {
			t.Fatalf("type %q has no notifier block", v.Type)
		}
		v.LvlUp()
	}
}

func TestNotifierIdempotencyKeys(t *testing.T) {
	if got := NotifierDeliveryKey("s1"); got != "settlement-notifier:s1" {
		t.Errorf("delivery key = %q", got)
	}
	if got := NotifierReissueKey("s1", 3); got != "settlement-notifier:s1:3" {
		t.Errorf("reissue key = %q", got)
	}
}

func TestResolveCoordinates(t *testing.T) {
	submitted := Vector2{X: 10, Y: 20}
	placement := &NotifierPlacement{Position: Vector3{X: 100, Y: 64, Z: -300}}

	t.Run("no notifier keeps submitted", func(t *testing.T) {
		got, locked := ResolveCoordinates(submitted, nil)
		if got != submitted || locked {
			t.Fatalf("got (%+v, %v), want (%+v, false)", got, locked, submitted)
		}
	})

	t.Run("notifier overrides differing input with world X and Z", func(t *testing.T) {
		got, locked := ResolveCoordinates(submitted, placement)
		want := Vector2{X: 100, Y: -300}
		if got != want || !locked {
			t.Fatalf("got (%+v, %v), want (%+v, true)", got, locked, want)
		}
	})

	t.Run("notifier wins even when input matches", func(t *testing.T) {
		got, locked := ResolveCoordinates(Vector2{X: 100, Y: -300}, placement)
		if got != (Vector2{X: 100, Y: -300}) || !locked {
			t.Fatalf("got (%+v, %v)", got, locked)
		}
	})
}

func TestRecipientSelection(t *testing.T) {
	s := &Settlement{Members: []Member{
		{UserId: "owner1", RoleIds: []string{OwnerRoleId}},
		{UserId: "plain", RoleIds: []string{}},
		{UserId: "owner2", RoleIds: []string{"r1", OwnerRoleId}},
		{UserId: "recruiter", RoleIds: []string{"r1"}},
	}}

	if got, want := s.OwnerIds(), []string{"owner1", "owner2"}; !slices.Equal(got, want) {
		t.Errorf("OwnerIds = %v, want %v", got, want)
	}
	if got, want := s.MemberIds(), []string{"owner1", "plain", "owner2", "recruiter"}; !slices.Equal(got, want) {
		t.Errorf("MemberIds = %v, want %v", got, want)
	}
}

func TestNextNotifierReissueCountsUp(t *testing.T) {
	s := &Settlement{}
	if n := s.NextNotifierReissue(); n != 1 {
		t.Fatalf("first reissue = %d, want 1", n)
	}
	if n := s.NextNotifierReissue(); n != 2 || s.NotifierReissues != 2 {
		t.Fatalf("second reissue = %d (stored %d), want 2", n, s.NotifierReissues)
	}
}
