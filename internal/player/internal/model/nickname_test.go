package model

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidNickname(t *testing.T) {
	valid := []string{"a", "Veter", "A1", "a-b_c", "123456789012345"}

	for _, name := range valid {
		if !ValidNickname(name) {
			t.Errorf("ValidNickname(%q) = false, want true", name)
		}
	}

	// The game's tab list takes ASCII only, so a Cyrillic name - even though the
	// site shows it next to a Cyrillic description - is refused here as well.
	invalid := []string{"", "1234567890123456", "Veter!", "Veter say", " Veter", "Ветер"}

	for _, name := range invalid {
		if ValidNickname(name) {
			t.Errorf("ValidNickname(%q) = true, want false", name)
		}
	}
}

func TestSetNicknameKeepsTheOldNameAndStartsTheCooldown(t *testing.T) {
	p := Player{UserGameName: "Veter"}
	now := time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)

	if err := p.SetNickname("Dozhd", now); err != nil {
		t.Fatalf("SetNickname: %v", err)
	}

	if p.UserGameName != "Dozhd" {
		t.Errorf("nickname = %q, want Dozhd", p.UserGameName)
	}
	if p.PreviousNickname != "Veter" {
		t.Errorf("previous nickname = %q, want the old one kept", p.PreviousNickname)
	}
	if !p.LastNicknameChangedAt.Equal(now) {
		t.Errorf("recorded change = %v, want %v", p.LastNicknameChangedAt, now)
	}
	if p.CanChangeNickname(now) {
		t.Error("the player can change the nickname again at once, want the cooldown")
	}
}

func TestSetNicknameRefusesAndLeavesThePlayerAlone(t *testing.T) {
	now := time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		nickname string
		want     error
	}{
		{"empty", "", ErrNicknameInvalid},
		{"too long", strings.Repeat("a", 16), ErrNicknameInvalid},
		{"punctuation", "Dozhd!", ErrNicknameInvalid},
		{"already current", "Veter", ErrNicknameUnchanged},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := Player{UserGameName: "Veter", PreviousNickname: "Ranee"}

			err := p.SetNickname(tc.nickname, now)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}

			if p.UserGameName != "Veter" || p.PreviousNickname != "Ranee" {
				t.Errorf("a refused change wrote the player: %+v", p)
			}
			if !p.LastNicknameChangedAt.IsZero() {
				t.Errorf("a refused change started a cooldown at %v", p.LastNicknameChangedAt)
			}
		})
	}
}

func TestNicknameChangeWindow(t *testing.T) {
	now := time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)

	t.Run("a player who never changed has nothing to wait for", func(t *testing.T) {
		p := Player{}

		if !p.CanChangeNickname(now) {
			t.Error("CanChangeNickname = false, want true")
		}
		if !p.NicknameChangeAllowedAt().IsZero() {
			t.Errorf("allowed at %v, want the zero time", p.NicknameChangeAllowedAt())
		}
	})

	t.Run("a recent change holds the nickname", func(t *testing.T) {
		p := Player{LastNicknameChangedAt: now.Add(-time.Hour)}

		if p.CanChangeNickname(now) {
			t.Error("CanChangeNickname = true, want false")
		}

		want := now.Add(-time.Hour).Add(NicknameCooldown)
		if !p.NicknameChangeAllowedAt().Equal(want) {
			t.Errorf("allowed at %v, want %v", p.NicknameChangeAllowedAt(), want)
		}
	})

	t.Run("the cooldown ends exactly six months after the change", func(t *testing.T) {
		p := Player{LastNicknameChangedAt: now.Add(-NicknameCooldown)}

		if !p.CanChangeNickname(now) {
			t.Error("CanChangeNickname = false at the end of the cooldown, want true")
		}
	})
}

func TestGrantNicknameChangeCutsTheCooldownShort(t *testing.T) {
	now := time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)
	p := Player{UserGameName: "Veter", LastNicknameChangedAt: now.Add(-time.Hour)}

	if err := p.GrantNicknameChange(); err != nil {
		t.Fatalf("GrantNicknameChange: %v", err)
	}

	if !p.LastNicknameChangedAt.IsZero() {
		t.Errorf("recorded change = %v, want it cleared", p.LastNicknameChangedAt)
	}
	if !p.CanChangeNickname(now) {
		t.Error("the player still cannot rename after a grant")
	}

	// A second grant would tell the player about a permission they already had.
	if err := p.GrantNicknameChange(); !errors.Is(err, ErrNicknameAlreadyChangeable) {
		t.Errorf("second grant err = %v, want ErrNicknameAlreadyChangeable", err)
	}
}
