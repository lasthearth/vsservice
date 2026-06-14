package model

import "time"

const (
	InitialELO = 1000
	MinELO     = 100
)

// PlayerStats holds a player's accumulated statistics for a single season.
// All state changes go through the model's methods.
type PlayerStats struct {
	ID         string
	PlayerID   string
	PlayerName string
	Elo        int
	Wins       int
	Kills      int
	SeasonID   string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func NewPlayerStats(playerID, playerName, seasonID string) *PlayerStats {
	now := time.Now()
	return &PlayerStats{
		PlayerID:   playerID,
		PlayerName: playerName,
		Elo:        InitialELO,
		SeasonID:   seasonID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// ReconstitutePlayerStats rebuilds PlayerStats from persisted state. Repository use only.
func ReconstitutePlayerStats(id, playerID, playerName string, elo, wins, kills int, seasonID string, createdAt, updatedAt time.Time) *PlayerStats {
	return &PlayerStats{
		ID:         id,
		PlayerID:   playerID,
		PlayerName: playerName,
		Elo:        elo,
		Wins:       wins,
		Kills:      kills,
		SeasonID:   seasonID,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}
}

// SetELO updates the player's ELO, enforcing the minimum.
func (p *PlayerStats) SetELO(newELO int) {
	if newELO < MinELO {
		newELO = MinELO
	}
	p.Elo = newELO
}

// RecordWin increments the player's win counter.
func (p *PlayerStats) RecordWin() {
	p.Wins++
}

// AddKills accumulates kills from a single match.
func (p *PlayerStats) AddKills(kills int) {
	if kills > 0 {
		p.Kills += kills
	}
}
