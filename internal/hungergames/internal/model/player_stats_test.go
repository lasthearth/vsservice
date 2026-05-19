package model_test

import (
	"testing"

	"github.com/lasthearth/vsservice/internal/hungergames/internal/model"
)

func TestNewPlayerStats(t *testing.T) {
	s := model.NewPlayerStats("user-1", "Player1", "season-1")

	if s.PlayerID != "user-1" {
		t.Errorf("PlayerID = %q, want user-1", s.PlayerID)
	}
	if s.PlayerName != "Player1" {
		t.Errorf("PlayerName = %q, want Player1", s.PlayerName)
	}
	if s.SeasonID != "season-1" {
		t.Errorf("SeasonID = %q, want season-1", s.SeasonID)
	}
	if s.Elo != model.InitialELO {
		t.Errorf("Elo = %d, want %d", s.Elo, model.InitialELO)
	}
	if s.Wins != 0 {
		t.Errorf("Wins = %d, want 0", s.Wins)
	}
	if s.Kills != 0 {
		t.Errorf("Kills = %d, want 0", s.Kills)
	}
}

func TestPlayerStats_SetELO(t *testing.T) {
	tests := []struct {
		name    string
		newELO  int
		wantELO int
	}{
		{"normal value", 1200, 1200},
		{"exact minimum", model.MinELO, model.MinELO},
		{"below minimum clamps to MinELO", model.MinELO - 1, model.MinELO},
		{"zero clamps to MinELO", 0, model.MinELO},
		{"negative clamps to MinELO", -500, model.MinELO},
		{"high value accepted", 3000, 3000},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := model.NewPlayerStats("u", "p", "s")
			s.SetELO(tc.newELO)
			if s.Elo != tc.wantELO {
				t.Errorf("SetELO(%d): Elo = %d, want %d", tc.newELO, s.Elo, tc.wantELO)
			}
		})
	}
}

func TestPlayerStats_RecordWin(t *testing.T) {
	s := model.NewPlayerStats("u", "p", "s")

	s.RecordWin()
	if s.Wins != 1 {
		t.Errorf("after 1 win: Wins = %d, want 1", s.Wins)
	}

	s.RecordWin()
	s.RecordWin()
	if s.Wins != 3 {
		t.Errorf("after 3 wins: Wins = %d, want 3", s.Wins)
	}
}

func TestPlayerStats_AddKills(t *testing.T) {
	tests := []struct {
		name      string
		kills     int
		wantKills int
	}{
		{"positive kills added", 5, 5},
		{"zero kills ignored", 0, 0},
		{"negative kills ignored", -3, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := model.NewPlayerStats("u", "p", "s")
			s.AddKills(tc.kills)
			if s.Kills != tc.wantKills {
				t.Errorf("AddKills(%d): Kills = %d, want %d", tc.kills, s.Kills, tc.wantKills)
			}
		})
	}
}

func TestPlayerStats_AddKills_Accumulates(t *testing.T) {
	s := model.NewPlayerStats("u", "p", "s")
	s.AddKills(3)
	s.AddKills(5)
	s.AddKills(0)
	s.AddKills(-1)

	if s.Kills != 8 {
		t.Errorf("Kills = %d, want 8", s.Kills)
	}
}
