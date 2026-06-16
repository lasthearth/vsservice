package model

import (
	"testing"
	"time"
)

func TestGenerateCode(t *testing.T) {
	rc := GenerateCode("user-1", "Player1")

	if rc.PlayerID != "user-1" {
		t.Errorf("PlayerID = %v, want user-1", rc.PlayerID)
	}
	if rc.PlayerName != "Player1" {
		t.Errorf("PlayerName = %v, want Player1", rc.PlayerName)
	}
	if rc.Id != "" {
		t.Errorf("Id = %v, want empty", rc.Id)
	}
	if rc.CreatedAt.IsZero() {
		t.Errorf("CreatedAt = %v, want non-zero", rc.CreatedAt)
	}
	if len(rc.Code) != referralCodeLength {
		t.Errorf("len(Code) = %v, want %v", len(rc.Code), referralCodeLength)
	}
	for _, c := range rc.Code {
		isUpper := c >= 'A' && c <= 'Z'
		isDigit := c >= '0' && c <= '9'
		if !isUpper && !isDigit {
			t.Errorf("Code = %v contains invalid character %q, want uppercase alphanumeric", rc.Code, c)
		}
	}
}

func TestGenerateCode_Uniqueness(t *testing.T) {
	const n = 1000
	seen := make(map[string]struct{}, n)
	for range n {
		rc := GenerateCode("user-1", "Player1")
		if _, dup := seen[rc.Code]; dup {
			t.Fatalf("GenerateCode produced duplicate code %q within %d iterations", rc.Code, n)
		}
		seen[rc.Code] = struct{}{}
	}
}

func TestReconstituteReferralCode(t *testing.T) {
	createdAt := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	rc := ReconstituteReferralCode("id-1", "user-1", "Player1", "ABCD1234", createdAt)

	if rc.Id != "id-1" {
		t.Errorf("Id = %v, want id-1", rc.Id)
	}
	if rc.PlayerID != "user-1" {
		t.Errorf("PlayerID = %v, want user-1", rc.PlayerID)
	}
	if rc.PlayerName != "Player1" {
		t.Errorf("PlayerName = %v, want Player1", rc.PlayerName)
	}
	if rc.Code != "ABCD1234" {
		t.Errorf("Code = %v, want ABCD1234", rc.Code)
	}
	if !rc.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt = %v, want %v", rc.CreatedAt, createdAt)
	}
}

func TestNewReferralEvent(t *testing.T) {
	re := NewReferralEvent("referrer-1", "referee-1", 100)

	if re.ReferrerPlayerID != "referrer-1" {
		t.Errorf("ReferrerPlayerID = %v, want referrer-1", re.ReferrerPlayerID)
	}
	if re.RefereePlayerID != "referee-1" {
		t.Errorf("RefereePlayerID = %v, want referee-1", re.RefereePlayerID)
	}
	if re.CoinsAwarded != 100 {
		t.Errorf("CoinsAwarded = %v, want 100", re.CoinsAwarded)
	}
	if re.Id != "" {
		t.Errorf("Id = %v, want empty", re.Id)
	}
	if re.CreatedAt.IsZero() {
		t.Errorf("CreatedAt = %v, want non-zero", re.CreatedAt)
	}
}

func TestReconstituteReferralEvent(t *testing.T) {
	createdAt := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	re := ReconstituteReferralEvent("id-1", "referrer-1", "referee-1", 250, createdAt)

	if re.Id != "id-1" {
		t.Errorf("Id = %v, want id-1", re.Id)
	}
	if re.ReferrerPlayerID != "referrer-1" {
		t.Errorf("ReferrerPlayerID = %v, want referrer-1", re.ReferrerPlayerID)
	}
	if re.RefereePlayerID != "referee-1" {
		t.Errorf("RefereePlayerID = %v, want referee-1", re.RefereePlayerID)
	}
	if re.CoinsAwarded != 250 {
		t.Errorf("CoinsAwarded = %v, want 250", re.CoinsAwarded)
	}
	if !re.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt = %v, want %v", re.CreatedAt, createdAt)
	}
}
