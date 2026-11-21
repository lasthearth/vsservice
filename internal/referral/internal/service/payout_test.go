package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/referral/internal/model"
)

// The tests in this file drive PayoutWorker.Pass directly: it is one pass of
// the loop, and the loop itself is a ticker around it.

func TestPayoutWaitsForTheReferee(t *testing.T) {
	repo := newFakeRepo().withEvent(testReferrerID, testRefereeID, testReferrerReward, testRefereeReward)
	wallet := newFakeWallet()

	newWorker(t, repo, wallet).Pass(context.Background())

	if len(wallet.credits) != 0 {
		t.Errorf("ledger rows = %v, want none: the referee has neither verified nor joined", wallet.credits)
	}
	if len(repo.claimed) != 0 {
		t.Errorf("claimed %v, want nothing claimed", repo.claimed)
	}
}

func TestPayoutPaysTheVerificationShare(t *testing.T) {
	repo := newFakeRepo().withEvent(testReferrerID, testRefereeID, testReferrerReward, testRefereeReward)
	repo.verified[testRefereeID] = true
	wallet := newFakeWallet()
	worker := newWorker(t, repo, wallet)

	worker.Pass(context.Background())

	if got := wallet.balances[testReferrerID]; got != 50 {
		t.Errorf("referrer balance = %d, want 50: a quarter of %d", got, testReferrerReward)
	}
	if got := wallet.balances[testRefereeID]; got != 0 {
		t.Errorf("referee balance = %d, want 0: their reward waits for the join", got)
	}
	if len(wallet.credits) != 1 {
		t.Fatalf("ledger rows = %v, want exactly one", wallet.credits)
	}
	if wallet.credits[0].playerID != testReferrerID || wallet.credits[0].amount != 50 {
		t.Errorf("ledger row = %+v, want 50 coins to the referrer", wallet.credits[0])
	}

	event := repo.events[testRefereeID]
	if !event.VerificationPaid || event.JoinPaid || event.RefereePaid {
		t.Errorf("paid stages = %+v, want verification only", event)
	}
	if event.CoinsAwarded != 50 {
		t.Errorf("CoinsAwarded = %d, want 50", event.CoinsAwarded)
	}
}

func TestPayoutPaysEverythingOnTheFirstJoin(t *testing.T) {
	repo := newFakeRepo().withEvent(testReferrerID, testRefereeID, testReferrerReward, testRefereeReward)
	repo.verified[testRefereeID] = true
	repo.joined[testRefereeID] = true
	wallet := newFakeWallet()
	worker := newWorker(t, repo, wallet)

	worker.Pass(context.Background())

	if got := wallet.balances[testReferrerID]; got != testReferrerReward {
		t.Errorf("referrer balance = %d, want the full %d", got, testReferrerReward)
	}
	if got := wallet.balances[testRefereeID]; got != testRefereeReward {
		t.Errorf("referee balance = %d, want %d", got, testRefereeReward)
	}
	if len(wallet.credits) != 3 {
		t.Fatalf("ledger rows = %v, want three: two shares and the referee's reward", wallet.credits)
	}

	event := repo.events[testRefereeID]
	if !event.VerificationPaid || !event.JoinPaid || !event.RefereePaid {
		t.Errorf("paid stages = %+v, want all three", event)
	}
	if event.CoinsAwarded != testReferrerReward {
		t.Errorf("CoinsAwarded = %d, want %d", event.CoinsAwarded, testReferrerReward)
	}
}

func TestPayoutPaysTheVerificationShareWhenOnlyTheJoinIsSeen(t *testing.T) {
	// The game server only writes presence for a player it let in, so a join is
	// proof of verification even when the payout loop never saw the approval.
	repo := newFakeRepo().withEvent(testReferrerID, testRefereeID, testReferrerReward, testRefereeReward)
	repo.joined[testRefereeID] = true
	wallet := newFakeWallet()

	newWorker(t, repo, wallet).Pass(context.Background())

	if got := wallet.balances[testReferrerID]; got != testReferrerReward {
		t.Errorf("referrer balance = %d, want the full %d", got, testReferrerReward)
	}
	if got := wallet.balances[testRefereeID]; got != testRefereeReward {
		t.Errorf("referee balance = %d, want %d", got, testRefereeReward)
	}
}

// TestPayoutTrustsTheClaimOverItsOwnSnapshot is the double-pay guard across two
// instances: the event a pass works from is a snapshot, and another instance may
// have paid a stage after it was read. Only the claim says who pays.
func TestPayoutTrustsTheClaimOverItsOwnSnapshot(t *testing.T) {
	repo := newFakeRepo().withEvent(testReferrerID, testRefereeID, testReferrerReward, testRefereeReward)
	repo.verified[testRefereeID] = true
	repo.joined[testRefereeID] = true
	wallet := newFakeWallet()
	worker := newWorker(t, repo, wallet)

	worker.Pass(context.Background())
	rowsAfterFirstPass := len(wallet.credits)

	// A snapshot taken before that pass claimed anything: it believes every
	// stage is still owed.
	stale := model.NewReferralEvent(testReferrerID, testRefereeID, testReferrerReward, testRefereeReward)

	worker.payEvent(context.Background(), testLogger(t), stale, time.Now())

	if len(wallet.credits) != rowsAfterFirstPass {
		t.Errorf("ledger rows = %d after a stale snapshot, want %d: every stage was already claimed",
			len(wallet.credits), rowsAfterFirstPass)
	}
	if got := wallet.balances[testReferrerID]; got != testReferrerReward {
		t.Errorf("referrer balance = %d, want %d", got, testReferrerReward)
	}
	if got := wallet.balances[testRefereeID]; got != testRefereeReward {
		t.Errorf("referee balance = %d, want %d", got, testRefereeReward)
	}
}

func TestPayoutPaysNothingTwice(t *testing.T) {
	repo := newFakeRepo().withEvent(testReferrerID, testRefereeID, testReferrerReward, testRefereeReward)
	repo.verified[testRefereeID] = true
	repo.joined[testRefereeID] = true
	wallet := newFakeWallet()
	worker := newWorker(t, repo, wallet)

	worker.Pass(context.Background())
	worker.Pass(context.Background())
	worker.Pass(context.Background())

	if got := wallet.balances[testReferrerID]; got != testReferrerReward {
		t.Errorf("referrer balance = %d after three passes, want %d", got, testReferrerReward)
	}
	if got := wallet.balances[testRefereeID]; got != testRefereeReward {
		t.Errorf("referee balance = %d after three passes, want %d", got, testRefereeReward)
	}
	if len(wallet.credits) != 3 {
		t.Errorf("ledger rows = %d after three passes, want 3", len(wallet.credits))
	}
}

func TestPayoutPaysOnceWhenTwoWorkersRunAtTheSameTime(t *testing.T) {
	repo := newFakeRepo().withEvent(testReferrerID, testRefereeID, testReferrerReward, testRefereeReward)
	repo.verified[testRefereeID] = true
	repo.joined[testRefereeID] = true
	wallet := newFakeWallet()

	// Two instances share one database. The claim is what keeps them from paying
	// the same stage twice.
	newWorker(t, repo, wallet).Pass(context.Background())
	newWorker(t, repo, wallet).Pass(context.Background())

	if got := wallet.balances[testReferrerID]; got != testReferrerReward {
		t.Errorf("referrer balance = %d, want %d", got, testReferrerReward)
	}
	if got := wallet.balances[testRefereeID]; got != testRefereeReward {
		t.Errorf("referee balance = %d, want %d", got, testRefereeReward)
	}
}

func TestPayoutRetriesAFailedReward(t *testing.T) {
	repo := newFakeRepo().withEvent(testReferrerID, testRefereeID, testReferrerReward, testRefereeReward)
	repo.verified[testRefereeID] = true
	wallet := newFakeWallet()
	wallet.addCoinsErr[testReferrerID] = errors.New("mongo is down")
	worker := newWorker(t, repo, wallet)

	worker.Pass(context.Background())

	if len(wallet.credits) != 0 {
		t.Errorf("ledger rows = %v, want none: the wallet refused the coins", wallet.credits)
	}
	if len(repo.released) != 1 {
		t.Fatalf("released %v, want the verification stage released for a retry", repo.released)
	}
	if repo.released[0].stage != model.StageVerification {
		t.Errorf("released stage = %q, want %q", repo.released[0].stage, model.StageVerification)
	}

	event := repo.events[testRefereeID]
	if event.VerificationPaid {
		t.Error("VerificationPaid = true after a failed payout, want false")
	}
	if event.CoinsAwarded != 0 {
		t.Errorf("CoinsAwarded = %d after a failed payout, want 0", event.CoinsAwarded)
	}

	// The wallet comes back: the next pass pays the reward, once.
	delete(wallet.addCoinsErr, testReferrerID)
	worker.Pass(context.Background())
	worker.Pass(context.Background())

	if got := wallet.balances[testReferrerID]; got != 50 {
		t.Errorf("referrer balance = %d after the retry, want 50", got)
	}
	if len(wallet.credits) != 1 {
		t.Errorf("ledger rows = %d after the retry, want 1", len(wallet.credits))
	}
}

func TestPayoutLeavesTheStageClaimedWhenTheReleaseFails(t *testing.T) {
	repo := newFakeRepo().withEvent(testReferrerID, testRefereeID, testReferrerReward, testRefereeReward)
	repo.verified[testRefereeID] = true
	repo.releaseErr = errors.New("mongo is down")
	wallet := newFakeWallet()
	wallet.addCoinsErr[testReferrerID] = errors.New("mongo is down")
	worker := newWorker(t, repo, wallet)

	worker.Pass(context.Background())

	delete(wallet.addCoinsErr, testReferrerID)
	repo.releaseErr = nil
	worker.Pass(context.Background())

	// Nothing was paid and the claim still stands, so the loop does not pay it
	// blind: the reward needs a human.
	if got := wallet.balances[testReferrerID]; got != 0 {
		t.Errorf("referrer balance = %d, want 0: the claim was never released", got)
	}
	if !repo.events[testRefereeID].VerificationPaid {
		t.Error("VerificationPaid = false, want the claim to stand")
	}
}

func TestPayoutSkipsAnEventWrittenBeforeTheTwoStagePayout(t *testing.T) {
	// Such a document carries no referrer_reward: its coins were paid in full
	// when the code was applied, so the loop must not pay it again.
	repo := newFakeRepo().withEvent(testReferrerID, testRefereeID, 0, 0)
	repo.verified[testRefereeID] = true
	repo.joined[testRefereeID] = true
	wallet := newFakeWallet()

	newWorker(t, repo, wallet).Pass(context.Background())

	if len(wallet.credits) != 0 {
		t.Errorf("ledger rows = %v, want none", wallet.credits)
	}
}

func TestPayoutSkipsEventsWhenTheListFails(t *testing.T) {
	repo := newFakeRepo().withEvent(testReferrerID, testRefereeID, testReferrerReward, testRefereeReward)
	repo.listErr = errors.New("mongo is down")
	wallet := newFakeWallet()

	newWorker(t, repo, wallet).Pass(context.Background())

	if len(wallet.credits) != 0 {
		t.Errorf("ledger rows = %v, want none", wallet.credits)
	}
}

func TestPayoutSkipsAnEventWhoseProgressCannotBeRead(t *testing.T) {
	repo := newFakeRepo().withEvent(testReferrerID, testRefereeID, testReferrerReward, testRefereeReward)
	repo.verified[testRefereeID] = true
	repo.progressErr = errors.New("mongo is down")
	wallet := newFakeWallet()

	newWorker(t, repo, wallet).Pass(context.Background())

	if len(wallet.credits) != 0 {
		t.Errorf("ledger rows = %v, want none", wallet.credits)
	}
	if len(repo.claimed) != 0 {
		t.Errorf("claimed %v, want nothing claimed", repo.claimed)
	}
}

func TestPayoutCreditReasonNamesTheReferee(t *testing.T) {
	repo := newFakeRepo().withEvent(testReferrerID, testRefereeID, testReferrerReward, testRefereeReward)
	repo.joined[testRefereeID] = true
	wallet := newFakeWallet()

	newWorker(t, repo, wallet).Pass(context.Background())

	for _, c := range wallet.credits {
		if c.reason == "" {
			t.Errorf("ledger row for %s has no reason", c.playerID)
		}
	}
}

func TestPayoutDefersAnEventTheRefereeHasNotReached(t *testing.T) {
	repo := newFakeRepo().withEvent(testReferrerID, testRefereeID, testReferrerReward, testRefereeReward)
	wallet := newFakeWallet()
	worker := newWorker(t, repo, wallet)
	now := time.Now()
	worker.now = func() time.Time { return now }

	worker.Pass(context.Background())

	event := repo.events[testRefereeID]
	if event.Checks != 1 {
		t.Errorf("Checks = %d after one fruitless pass, want 1", event.Checks)
	}
	if !event.NextCheckAt.After(now) {
		t.Errorf("NextCheckAt = %v, want a moment after %v", event.NextCheckAt, now)
	}
	if len(repo.deferred) != 1 {
		t.Fatalf("deferred %v, want the one event deferred", repo.deferred)
	}

	// The next tick comes long before that moment: the event is out of the way,
	// and a referral that just moved is not queued behind it.
	worker.Pass(context.Background())

	if event.Checks != 1 {
		t.Errorf("Checks = %d after a second immediate pass, want 1: the event was deferred", event.Checks)
	}
	if len(wallet.credits) != 0 {
		t.Errorf("ledger rows = %v, want none", wallet.credits)
	}
}

func TestPayoutDoesNotDeferWhenItPays(t *testing.T) {
	repo := newFakeRepo().withEvent(testReferrerID, testRefereeID, testReferrerReward, testRefereeReward)
	repo.verified[testRefereeID] = true
	wallet := newFakeWallet()

	newWorker(t, repo, wallet).Pass(context.Background())

	if len(repo.deferred) != 0 {
		t.Errorf("deferred %v, want nothing: the pass paid a stage", repo.deferred)
	}
	if repo.events[testRefereeID].Checks != 0 {
		t.Errorf("Checks = %d, want 0", repo.events[testRefereeID].Checks)
	}
}
