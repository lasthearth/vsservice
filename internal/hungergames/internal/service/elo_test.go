package service_test

import (
	"testing"

	"github.com/lasthearth/vsservice/internal/hungergames/internal/service"
)

func TestCalculateELO_TwoPlayers(t *testing.T) {
	tests := []struct {
		name     string
		p1ELO    int
		p2ELO    int
		p1Place  int
		p2Place  int
		wantP1Up bool // p1 ELO should increase
		wantP2Up bool // p2 ELO should increase
	}{
		{
			name:  "equal players, p1 wins — p1 gains, p2 loses",
			p1ELO: 1000, p2ELO: 1000,
			p1Place: 1, p2Place: 2,
			wantP1Up: true, wantP2Up: false,
		},
		{
			name:  "equal players, p2 wins — p2 gains, p1 loses",
			p1ELO: 1000, p2ELO: 1000,
			p1Place: 2, p2Place: 1,
			wantP1Up: false, wantP2Up: true,
		},
		{
			name:  "favourite wins — smaller gain than underdog win",
			p1ELO: 1200, p2ELO: 800,
			p1Place: 1, p2Place: 2,
			wantP1Up: true, wantP2Up: false,
		},
		{
			name:  "underdog wins — larger gain",
			p1ELO: 800, p2ELO: 1200,
			p1Place: 1, p2Place: 2,
			wantP1Up: true, wantP2Up: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			players := []service.PlayerPlacement{
				{PlayerID: "p1", Place: tc.p1Place, CurrentELO: tc.p1ELO},
				{PlayerID: "p2", Place: tc.p2Place, CurrentELO: tc.p2ELO},
			}

			results := service.CalculateELO(players)
			if len(results) != 2 {
				t.Fatalf("expected 2 results, got %d", len(results))
			}

			byID := make(map[string]service.ELOResult)
			for _, r := range results {
				byID[r.PlayerID] = r
			}

			if tc.wantP1Up && byID["p1"].NewELO <= tc.p1ELO {
				t.Errorf("p1 ELO should increase: got %d, was %d", byID["p1"].NewELO, tc.p1ELO)
			}
			if !tc.wantP1Up && byID["p1"].NewELO >= tc.p1ELO {
				t.Errorf("p1 ELO should decrease: got %d, was %d", byID["p1"].NewELO, tc.p1ELO)
			}
			if tc.wantP2Up && byID["p2"].NewELO <= tc.p2ELO {
				t.Errorf("p2 ELO should increase: got %d, was %d", byID["p2"].NewELO, tc.p2ELO)
			}
			if !tc.wantP2Up && byID["p2"].NewELO >= tc.p2ELO {
				t.Errorf("p2 ELO should decrease: got %d, was %d", byID["p2"].NewELO, tc.p2ELO)
			}
		})
	}
}

func TestCalculateELO_UnderdogGainsMore(t *testing.T) {
	equal := []service.PlayerPlacement{
		{PlayerID: "a", Place: 1, CurrentELO: 1000},
		{PlayerID: "b", Place: 2, CurrentELO: 1000},
	}
	underdog := []service.PlayerPlacement{
		{PlayerID: "a", Place: 1, CurrentELO: 800},
		{PlayerID: "b", Place: 2, CurrentELO: 1200},
	}

	eqResults := eloByID(service.CalculateELO(equal))
	udResults := eloByID(service.CalculateELO(underdog))

	gainEqual := eqResults["a"] - 1000
	gainUnderdog := udResults["a"] - 800

	if gainUnderdog <= gainEqual {
		t.Errorf("underdog gain (%d) should exceed equal-match gain (%d)", gainUnderdog, gainEqual)
	}
}

func TestCalculateELO_MinELOClamp(t *testing.T) {
	players := []service.PlayerPlacement{
		{PlayerID: "strong", Place: 1, CurrentELO: 1000},
		{PlayerID: "weak", Place: 2, CurrentELO: 101}, // one loss should not go below MinELO
	}

	results := eloByID(service.CalculateELO(players))

	if results["weak"] < 100 {
		t.Errorf("ELO below minimum: got %d, want >= 100", results["weak"])
	}
}

func TestCalculateELO_ZeroSumApprox(t *testing.T) {
	// Total ELO change across all players should be approximately zero
	// (small rounding differences from math.Round are acceptable).
	players := []service.PlayerPlacement{
		{PlayerID: "a", Place: 1, CurrentELO: 1000},
		{PlayerID: "b", Place: 2, CurrentELO: 1000},
		{PlayerID: "c", Place: 3, CurrentELO: 1000},
		{PlayerID: "d", Place: 4, CurrentELO: 1000},
	}

	results := service.CalculateELO(players)

	var totalBefore, totalAfter int
	for i, r := range results {
		totalBefore += players[i].CurrentELO
		totalAfter += r.NewELO
	}

	diff := totalAfter - totalBefore
	if diff < -len(players) || diff > len(players) {
		t.Errorf("ELO not approximately zero-sum: delta = %d", diff)
	}
}

func TestCalculateELO_MultiPlayer(t *testing.T) {
	players := []service.PlayerPlacement{
		{PlayerID: "1st", Place: 1, CurrentELO: 1000},
		{PlayerID: "2nd", Place: 2, CurrentELO: 1000},
		{PlayerID: "3rd", Place: 3, CurrentELO: 1000},
	}

	results := eloByID(service.CalculateELO(players))

	if results["1st"] <= results["2nd"] {
		t.Errorf("1st (%d) should have higher ELO than 2nd (%d)", results["1st"], results["2nd"])
	}
	if results["2nd"] <= results["3rd"] {
		t.Errorf("2nd (%d) should have higher ELO than 3rd (%d)", results["2nd"], results["3rd"])
	}
}

func TestCalculateELO_Tie(t *testing.T) {
	// Same place = draw: ELO should not change for equal players
	players := []service.PlayerPlacement{
		{PlayerID: "a", Place: 1, CurrentELO: 1000},
		{PlayerID: "b", Place: 1, CurrentELO: 1000},
	}

	results := eloByID(service.CalculateELO(players))

	if results["a"] != 1000 || results["b"] != 1000 {
		t.Errorf("equal players with same place should not change ELO: a=%d b=%d", results["a"], results["b"])
	}
}

func TestCalculateELO_SinglePlayer(t *testing.T) {
	players := []service.PlayerPlacement{
		{PlayerID: "solo", Place: 1, CurrentELO: 1500},
	}

	results := service.CalculateELO(players)

	if len(results) != 1 || results[0].NewELO != 1500 {
		t.Errorf("single player ELO should be unchanged: got %d", results[0].NewELO)
	}
}

// eloByID converts []ELOResult to map[playerID]newELO for easier assertions.
func eloByID(results []service.ELOResult) map[string]int {
	m := make(map[string]int, len(results))
	for _, r := range results {
		m[r.PlayerID] = r.NewELO
	}
	return m
}
