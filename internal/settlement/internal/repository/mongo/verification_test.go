package repository

import (
	"testing"

	"github.com/lasthearth/vsservice/internal/settlement/model"
)

// Approve must not write the request's coordinates over the settlement's while a
// notifier block stands. The block is authoritative, so a block placed or moved
// between Submit and Approve keeps its position and the map pin keeps agreeing
// with notifier_position.
func TestApprovalCoordinatesFollowTheStandingNotifier(t *testing.T) {
	submitted := model.Vector2{X: 10, Y: 20}
	placement := &model.NotifierPlacement{
		SettlementId: "s1",
		Position:     model.Vector3{X: 100, Y: 64, Z: -300},
	}

	// World X and Z are the site's X and Y; the submitted value is dropped.
	if got, want := approvalCoordinates(submitted, placement), (model.Vector2{X: 100, Y: -300}); got != want {
		t.Errorf("with a notifier = %+v, want %+v", got, want)
	}
}

// With no block placed the client's coordinates are all there is, so the
// approval keeps them. This is the create path of a first submission and the
// update path of a settlement that never had a block.
func TestApprovalCoordinatesWithoutNotifierKeepTheSubmittedOnes(t *testing.T) {
	submitted := model.Vector2{X: 10, Y: 20}

	if got := approvalCoordinates(submitted, nil); got != submitted {
		t.Errorf("without a notifier = %+v, want the submitted %+v", got, submitted)
	}
}

// The authority decides, not the difference: a block that already stands where
// the client asked for resolves to the same value and stays locked.
func TestApprovalCoordinatesMatchKeepsTheNotifierValue(t *testing.T) {
	same := model.Vector2{X: 100, Y: -300}
	placement := &model.NotifierPlacement{Position: model.Vector3{X: 100, Y: 64, Z: -300}}

	if got := approvalCoordinates(same, placement); got != same {
		t.Errorf("matching input = %+v, want %+v", got, same)
	}
}
