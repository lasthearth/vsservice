package service

import "math"

const (
	eloKFactor = 32
)

// PlayerPlacement carries the data needed to compute an ELO update for one player.
type PlayerPlacement struct {
	PlayerID   string
	Place      int // 1-based; 1 = winner
	CurrentELO int
}

// ELOResult holds the post-match ELO for a single player.
type ELOResult struct {
	PlayerID string
	NewELO   int
}

// CalculateELO computes new ELO ratings for a multi-player match using pairwise
// comparison. For every ordered pair (i, j) where i placed higher than j:
//
//	expected_i = 1 / (1 + 10^((elo_j - elo_i) / 400))
//	delta_i   += K * (1 - expected_i)
//	delta_j   += K * (0 - expected_j)
//
// Deltas are normalised by dividing by (N-1) to prevent runaway ELO swings in
// large lobbies. The result is clamped to model.MinELO.
//
// Ties (identical place values) are treated as a draw (actual score = 0.5).
func CalculateELO(players []PlayerPlacement) []ELOResult {
	n := len(players)
	if n < 2 {
		results := make([]ELOResult, n)
		for i, p := range players {
			results[i] = ELOResult{PlayerID: p.PlayerID, NewELO: p.CurrentELO}
		}
		return results
	}

	deltas := make(map[string]float64, n)

	for i := range n {
		for j := i + 1; j < n; j++ {
			pi, pj := players[i], players[j]

			var actualI float64
			switch {
			case pi.Place < pj.Place:
				actualI = 1.0
			case pi.Place > pj.Place:
				actualI = 0.0
			default:
				actualI = 0.5 // tie
			}
			actualJ := 1.0 - actualI

			expectedI := 1.0 / (1.0 + math.Pow(10, float64(pj.CurrentELO-pi.CurrentELO)/400.0))
			expectedJ := 1.0 - expectedI

			deltas[pi.PlayerID] += eloKFactor * (actualI - expectedI)
			deltas[pj.PlayerID] += eloKFactor * (actualJ - expectedJ)
		}
	}

	norm := float64(n - 1)
	results := make([]ELOResult, n)
	for i, p := range players {
		delta := deltas[p.PlayerID] / norm
		newELO := max(p.CurrentELO+int(math.Round(delta)), minELO)
		results[i] = ELOResult{PlayerID: p.PlayerID, NewELO: newELO}
	}
	return results
}

// minELO mirrors model.MinELO without importing the model package from elo.go.
const minELO = 100
