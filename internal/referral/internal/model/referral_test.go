package model

import (
	"errors"
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
	re := NewReferralEvent("referrer-1", "referee-1", 200, 100)

	if re.ReferrerPlayerID != "referrer-1" {
		t.Errorf("ReferrerPlayerID = %v, want referrer-1", re.ReferrerPlayerID)
	}
	if re.RefereePlayerID != "referee-1" {
		t.Errorf("RefereePlayerID = %v, want referee-1", re.RefereePlayerID)
	}
	if re.ReferrerReward != 200 {
		t.Errorf("ReferrerReward = %v, want 200", re.ReferrerReward)
	}
	if re.RefereeReward != 100 {
		t.Errorf("RefereeReward = %v, want 100", re.RefereeReward)
	}
	if re.CoinsAwarded != 0 {
		t.Errorf("CoinsAwarded = %v, want 0: a new event owes everything", re.CoinsAwarded)
	}
	if re.VerificationPaid || re.JoinPaid || re.RefereePaid {
		t.Errorf("paid stages = %+v, want none", re)
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

	nextCheckAt := createdAt.Add(time.Hour)

	re := ReconstituteReferralEvent("id-1", "referrer-1", "referee-1", 200, 100, 50, true, false, false, 3, nextCheckAt, createdAt)

	if re.Id != "id-1" {
		t.Errorf("Id = %v, want id-1", re.Id)
	}
	if re.ReferrerPlayerID != "referrer-1" {
		t.Errorf("ReferrerPlayerID = %v, want referrer-1", re.ReferrerPlayerID)
	}
	if re.RefereePlayerID != "referee-1" {
		t.Errorf("RefereePlayerID = %v, want referee-1", re.RefereePlayerID)
	}
	if re.ReferrerReward != 200 {
		t.Errorf("ReferrerReward = %v, want 200", re.ReferrerReward)
	}
	if re.RefereeReward != 100 {
		t.Errorf("RefereeReward = %v, want 100", re.RefereeReward)
	}
	if re.CoinsAwarded != 50 {
		t.Errorf("CoinsAwarded = %v, want 50", re.CoinsAwarded)
	}
	if !re.VerificationPaid || re.JoinPaid || re.RefereePaid {
		t.Errorf("paid stages = %+v, want verification only", re)
	}
	if re.Checks != 3 {
		t.Errorf("Checks = %d, want 3", re.Checks)
	}
	if !re.NextCheckAt.Equal(nextCheckAt) {
		t.Errorf("NextCheckAt = %v, want %v", re.NextCheckAt, nextCheckAt)
	}
	if !re.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt = %v, want %v", re.CreatedAt, createdAt)
	}
}

func TestSharesAddUpToTheFullReward(t *testing.T) {
	tests := []struct {
		name         string
		reward       int64
		wantVerified int64
		wantJoin     int64
	}{
		{name: "even split", reward: 200, wantVerified: 50, wantJoin: 150},
		{name: "the default reward", reward: 100, wantVerified: 25, wantJoin: 75},
		{name: "rounding goes to the join share", reward: 101, wantVerified: 25, wantJoin: 76},
		{name: "a share below one coin is not paid", reward: 3, wantVerified: 0, wantJoin: 3},
		{name: "rounding of a small reward", reward: 7, wantVerified: 1, wantJoin: 6},
		{name: "a zero reward pays nothing", reward: 0, wantVerified: 0, wantJoin: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := NewReferralEvent("referrer-1", "referee-1", tt.reward, 0)

			if got := event.VerificationShare(); got != tt.wantVerified {
				t.Errorf("VerificationShare() = %d, want %d", got, tt.wantVerified)
			}
			if got := event.JoinShare(); got != tt.wantJoin {
				t.Errorf("JoinShare() = %d, want %d", got, tt.wantJoin)
			}
			if got := event.VerificationShare() + event.JoinShare(); got != tt.reward {
				t.Errorf("the two shares add up to %d, want the full reward %d", got, tt.reward)
			}
		})
	}
}

func TestPendingRewardsFollowTheReferee(t *testing.T) {
	tests := []struct {
		name       string
		verified   bool
		joined     bool
		wantStages []RewardStage
		wantCoins  []int64
		wantPayees []string
	}{
		{
			name: "a referee that has done nothing is owed nothing",
		},
		{
			name:       "verification pays the first share",
			verified:   true,
			wantStages: []RewardStage{StageVerification},
			wantCoins:  []int64{50},
			wantPayees: []string{"referrer-1"},
		},
		{
			name:       "a join pays everything still owed",
			verified:   true,
			joined:     true,
			wantStages: []RewardStage{StageVerification, StageJoin, StageReferee},
			wantCoins:  []int64{50, 150, 100},
			wantPayees: []string{"referrer-1", "referrer-1", "referee-1"},
		},
		{
			name:       "a join seen before the approval pays the verification share too",
			joined:     true,
			wantStages: []RewardStage{StageVerification, StageJoin, StageReferee},
			wantCoins:  []int64{50, 150, 100},
			wantPayees: []string{"referrer-1", "referrer-1", "referee-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := NewReferralEvent("referrer-1", "referee-1", 200, 100)

			rewards := event.PendingRewards(tt.verified, tt.joined)

			if len(rewards) != len(tt.wantStages) {
				t.Fatalf("PendingRewards() = %v, want %d rewards", rewards, len(tt.wantStages))
			}
			for i, reward := range rewards {
				if reward.Stage != tt.wantStages[i] {
					t.Errorf("reward %d stage = %q, want %q", i, reward.Stage, tt.wantStages[i])
				}
				if reward.Coins != tt.wantCoins[i] {
					t.Errorf("reward %d coins = %d, want %d", i, reward.Coins, tt.wantCoins[i])
				}
				if reward.Payee != tt.wantPayees[i] {
					t.Errorf("reward %d payee = %q, want %q", i, reward.Payee, tt.wantPayees[i])
				}
			}
		})
	}
}

func TestPendingRewardsSkipsPaidAndZeroStages(t *testing.T) {
	event := NewReferralEvent("referrer-1", "referee-1", 200, 0)
	if err := event.MarkPaid(StageVerification); err != nil {
		t.Fatalf("MarkPaid() error = %v", err)
	}

	rewards := event.PendingRewards(true, true)

	if len(rewards) != 1 {
		t.Fatalf("PendingRewards() = %v, want the join share only", rewards)
	}
	if rewards[0].Stage != StageJoin || rewards[0].Coins != 150 {
		t.Errorf("reward = %+v, want the join share of 150", rewards[0])
	}
}

func TestMarkPaidCountsOnce(t *testing.T) {
	event := NewReferralEvent("referrer-1", "referee-1", 200, 100)

	if err := event.MarkPaid(StageVerification); err != nil {
		t.Fatalf("first MarkPaid() error = %v", err)
	}
	if event.CoinsAwarded != 50 {
		t.Errorf("CoinsAwarded = %d, want 50", event.CoinsAwarded)
	}

	if err := event.MarkPaid(StageVerification); !errors.Is(err, ErrStageAlreadyPaid) {
		t.Fatalf("second MarkPaid() error = %v, want ErrStageAlreadyPaid", err)
	}
	if event.CoinsAwarded != 50 {
		t.Errorf("CoinsAwarded = %d after the refused claim, want 50", event.CoinsAwarded)
	}

	if err := event.MarkPaid(StageReferee); err != nil {
		t.Fatalf("MarkPaid(StageReferee) error = %v", err)
	}
	if event.CoinsAwarded != 50 {
		t.Errorf("CoinsAwarded = %d, want 50: the referee's reward is not the referrer's total", event.CoinsAwarded)
	}
}

func TestUnmarkPaidRestoresTheTotal(t *testing.T) {
	event := NewReferralEvent("referrer-1", "referee-1", 200, 100)

	if err := event.UnmarkPaid(StageJoin); !errors.Is(err, ErrStageNotPaid) {
		t.Fatalf("UnmarkPaid() on an unpaid stage error = %v, want ErrStageNotPaid", err)
	}

	if err := event.MarkPaid(StageJoin); err != nil {
		t.Fatalf("MarkPaid() error = %v", err)
	}
	if event.CoinsAwarded != 150 {
		t.Errorf("CoinsAwarded = %d, want 150", event.CoinsAwarded)
	}

	if err := event.UnmarkPaid(StageJoin); err != nil {
		t.Fatalf("UnmarkPaid() error = %v", err)
	}
	if event.CoinsAwarded != 0 {
		t.Errorf("CoinsAwarded = %d after the release, want 0", event.CoinsAwarded)
	}
	if event.JoinPaid {
		t.Error("JoinPaid = true after the release, want false")
	}
}

func TestMarkPaidRefusesAnUnknownStage(t *testing.T) {
	event := NewReferralEvent("referrer-1", "referee-1", 200, 100)

	if err := event.MarkPaid(RewardStage("nonsense")); err == nil {
		t.Fatal("MarkPaid() error = nil, want an error for an unknown stage")
	}
}

func TestDeferNextBacksOff(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	event := ReconstituteReferralEvent("id-1", "referrer-1", "referee-1", 200, 100, 0, false, false, false, 0, now, now)

	want := []time.Duration{
		time.Minute,
		5 * time.Minute,
		15 * time.Minute,
		30 * time.Minute,
		time.Hour,
		time.Hour,
	}

	for i, delay := range want {
		until := event.DeferNext(now)

		if got := until.Sub(now); got != delay {
			t.Errorf("check %d waits %v, want %v", i+1, got, delay)
		}
		if event.Checks != i+1 {
			t.Errorf("Checks = %d after check %d, want %d", event.Checks, i+1, i+1)
		}
		if !until.Equal(event.NextCheckAt) {
			t.Errorf("returned %v but stored %v", until, event.NextCheckAt)
		}
	}
}
