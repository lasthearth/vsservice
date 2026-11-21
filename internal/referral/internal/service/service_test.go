package service

import (
	"errors"
	"testing"

	"github.com/lasthearth/vsservice/internal/referral/internal/ierror"
	"github.com/lasthearth/vsservice/internal/referral/internal/model"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestUseReferralCodeRecordsTheRewardsAndPaysNothingYet(t *testing.T) {
	repo := newFakeRepo().withCode(testReferrerID, testCode)
	wallet := newFakeWallet()
	svc := newService(t, repo)

	resp, err := svc.UseReferralCode(refereeCtx(), useCode(testCode))
	if err != nil {
		t.Fatalf("UseReferralCode() error = %v, want nil", err)
	}
	if resp == nil {
		t.Fatal("UseReferralCode() response = nil, want an empty message")
	}

	event, ok := repo.events[testRefereeID]
	if !ok {
		t.Fatal("no referral event was recorded")
	}
	if event.ReferrerPlayerID != testReferrerID {
		t.Errorf("ReferrerPlayerID = %v, want %v", event.ReferrerPlayerID, testReferrerID)
	}
	if event.ReferrerReward != testReferrerReward {
		t.Errorf("ReferrerReward = %v, want %v", event.ReferrerReward, testReferrerReward)
	}
	if event.RefereeReward != testRefereeReward {
		t.Errorf("RefereeReward = %v, want %v", event.RefereeReward, testRefereeReward)
	}
	if event.CoinsAwarded != 0 {
		t.Errorf("CoinsAwarded = %v, want 0: applying a code pays nobody", event.CoinsAwarded)
	}
	if event.VerificationPaid || event.JoinPaid || event.RefereePaid {
		t.Errorf("paid stages = %+v, want none", event)
	}
	if len(wallet.credits) != 0 {
		t.Errorf("ledger rows = %v, want none: applying a code pays nobody", wallet.credits)
	}
}

func TestUseReferralCodeRejectsSelfReferral(t *testing.T) {
	repo := newFakeRepo().withCode(testReferrerID, testCode)
	svc := newService(t, repo)

	ctx := refereeCtx()
	ctx = withUserID(ctx, testReferrerID)

	_, err := svc.UseReferralCode(ctx, useCode(testCode))
	if !errors.Is(err, ErrSelfReferral) {
		t.Fatalf("UseReferralCode() error = %v, want ErrSelfReferral", err)
	}
	if len(repo.events) != 0 {
		t.Errorf("recorded %d events, want 0", len(repo.events))
	}
}

func TestUseReferralCodeRejectsASecondCode(t *testing.T) {
	repo := newFakeRepo().
		withCode(testReferrerID, testCode).
		withCode(testOtherID, "ZZZZ9999")
	svc := newService(t, repo)

	if _, err := svc.UseReferralCode(refereeCtx(), useCode(testCode)); err != nil {
		t.Fatalf("first UseReferralCode() error = %v, want nil", err)
	}

	_, err := svc.UseReferralCode(refereeCtx(), useCode("ZZZZ9999"))
	if !errors.Is(err, ierror.ErrAlreadyReferred) {
		t.Fatalf("second UseReferralCode() error = %v, want ErrAlreadyReferred", err)
	}
}

func TestUseReferralCodeRejectsAnUnknownCode(t *testing.T) {
	svc := newService(t, newFakeRepo())

	_, err := svc.UseReferralCode(refereeCtx(), useCode("NOSUCHCODE"))
	if !errors.Is(err, ierror.ErrNotFound) {
		t.Fatalf("UseReferralCode() error = %v, want ErrNotFound", err)
	}
}

func TestGetMyReferralCodeReturnsTheSameCodeTwice(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(t, repo)

	ctx := withUserID(refereeCtx(), testReferrerID)

	first, err := svc.GetMyReferralCode(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("first GetMyReferralCode() error = %v", err)
	}
	second, err := svc.GetMyReferralCode(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("second GetMyReferralCode() error = %v", err)
	}

	if first.GetCode() == "" {
		t.Fatal("code is empty")
	}
	if first.GetCode() != second.GetCode() {
		t.Errorf("codes = %q and %q, want the same code", first.GetCode(), second.GetCode())
	}
}

func TestGetMyReferralStatsCountsOnlyPaidCoins(t *testing.T) {
	repo := newFakeRepo().
		withEvent(testReferrerID, testRefereeID, testReferrerReward, testRefereeReward).
		withEvent(testReferrerID, testOtherID, testReferrerReward, testRefereeReward)

	// One referee verified and nothing else: the referrer is owed 50 of 200 for
	// them, and the other referral has not moved at all.
	event := repo.events[testRefereeID]
	if err := event.MarkPaid(model.StageVerification); err != nil {
		t.Fatalf("MarkPaid() error = %v", err)
	}

	svc := newService(t, repo)
	ctx := withUserID(refereeCtx(), testReferrerID)

	resp, err := svc.GetMyReferralStats(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("GetMyReferralStats() error = %v", err)
	}

	if resp.GetTotalReferrals() != 2 {
		t.Errorf("TotalReferrals = %v, want 2", resp.GetTotalReferrals())
	}
	if resp.GetTotalCoinsEarned() != 50 {
		t.Errorf("TotalCoinsEarned = %v, want 50: only the paid share counts", resp.GetTotalCoinsEarned())
	}
}
