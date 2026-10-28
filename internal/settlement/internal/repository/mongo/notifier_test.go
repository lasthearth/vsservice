package repository

import (
	"testing"

	"github.com/lasthearth/vsservice/internal/settlement/model"
)

// One block is one pin. Binding a neutral herald writes its bound row and
// retires the neutral one, but the two writes are not atomic, and a retirement
// that never landed leaves the same position in both collections. The map must
// not draw the block twice, so the bound row wins: it names the settlement the
// pin belongs to.
func TestNeutralRowYieldsToABoundHeraldAtTheSamePosition(t *testing.T) {
	shared := model.Vector3{X: 7, Y: 64, Z: -300}
	bound := []model.NotifierPlacement{
		{SettlementId: "s1", Position: shared},
	}
	unbound := []model.NotifierPlacement{
		{Position: shared},
		{Position: model.Vector3{X: 21, Y: 66, Z: -4}},
	}

	got := withoutStandingTwice(bound, unbound)

	if len(got) != 1 {
		t.Fatalf("neutral rows = %+v, want only the one no bound herald occupies", got)
	}
	if got[0].Position != (model.Vector3{X: 21, Y: 66, Z: -4}) {
		t.Errorf("kept %+v, want the neutral herald at (21,66,-4)", got[0])
	}
}

// A neutral herald is a pin of its own, so nothing may drop it while no bound
// herald stands where it does — an equal X and Z at another height is another
// block, not the same one.
func TestNeutralRowsSurviveABoundHeraldElsewhere(t *testing.T) {
	bound := []model.NotifierPlacement{
		{SettlementId: "s1", Position: model.Vector3{X: 7, Y: 64, Z: -300}},
	}
	unbound := []model.NotifierPlacement{
		{Position: model.Vector3{X: 7, Y: 90, Z: -300}},
	}

	if got := withoutStandingTwice(bound, unbound); len(got) != 1 {
		t.Errorf("neutral rows = %+v, want the one kept", got)
	}
}

func TestWithoutStandingTwiceKeepsEmptyInputEmpty(t *testing.T) {
	if got := withoutStandingTwice(nil, nil); len(got) != 0 {
		t.Errorf("rows = %+v, want none", got)
	}

	bound := []model.NotifierPlacement{{SettlementId: "s1", Position: model.Vector3{X: 1, Y: 2, Z: 3}}}
	if got := withoutStandingTwice(bound, nil); len(got) != 0 {
		t.Errorf("rows = %+v, want none", got)
	}
}
