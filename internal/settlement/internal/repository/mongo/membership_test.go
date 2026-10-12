package repository

import (
	"errors"
	"testing"

	repoerr "github.com/lasthearth/vsservice/internal/settlement/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement/internal/repository/mongo/repomapper"
	"github.com/lasthearth/vsservice/internal/settlement/model"
)

// toSettlementDTO must stamp the kind on every write, because UpdateDoc replaces the whole document:
// a mapping that dropped the field would take the group out of both unique indexes and free its
// members to take a second membership of that kind.
func TestToSettlementDTOStampsTheKind(t *testing.T) {
	r := &Repository{mapper: &repomapper.MapperImpl{}}

	cases := []struct {
		typ  model.SettlementType
		kind string
	}{
		{model.SettlementTypeCamp, "settlement"},
		{model.SettlementTypeProvince, "settlement"},
		{model.SettlementTypeGuild, "guild"},
	}
	for _, tc := range cases {
		if got := r.toSettlementDTO(&model.Settlement{Type: tc.typ}); got.Kind != tc.kind {
			t.Errorf("type %q: kind = %q, want %q", tc.typ, got.Kind, tc.kind)
		}
	}
}

// The model owns the membership rules; the repository owns the mapping of what they answer to the
// domain errors the interceptor turns into gRPC codes.
func TestMembershipErrMapsTheModelRules(t *testing.T) {
	if got := membershipErr(nil); got != nil {
		t.Errorf("nil rule error = %v, want nil", got)
	}

	cases := []struct {
		in   error
		want error
	}{
		{model.ErrAlreadyInSettlement, repoerr.ErrAlreadyMember},
		{model.ErrAlreadyInGuild, repoerr.ErrAlreadyInGuild},
		{model.ErrGuildOwnerInSettlement, repoerr.ErrGuildOwnerInSettlement},
	}
	for _, tc := range cases {
		if got := membershipErr(tc.in); !errors.Is(got, tc.want) {
			t.Errorf("membershipErr(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}

	// Anything else passes through unchanged, so a storage failure is never mistaken for a rule.
	other := errors.New("mongo down")
	if got := membershipErr(other); !errors.Is(got, other) {
		t.Errorf("membershipErr(other) = %v, want it unchanged", got)
	}
}

// A duplicate key comes from one of the two per-kind indexes, and the answer must name the kind the
// player already holds.
func TestDuplicateMemberErrNamesTheKind(t *testing.T) {
	if got := duplicateMemberErr(model.KindGuild); !errors.Is(got, repoerr.ErrAlreadyInGuild) {
		t.Errorf("guild = %v, want ErrAlreadyInGuild", got)
	}
	if got := duplicateMemberErr(model.KindSettlement); !errors.Is(got, repoerr.ErrAlreadyMember) {
		t.Errorf("settlement = %v, want ErrAlreadyMember", got)
	}
}
