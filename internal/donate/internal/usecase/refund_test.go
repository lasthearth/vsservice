package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/lasthearth/vsservice/internal/donate/internal/ierror"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
)

func TestRefundActivePurchase(t *testing.T) {
	repo := newFakeRepo().withWallet("p1", "Bob", 70)
	repo.withPurchase("pu1", "p1", "Bob", 30)

	p, err := newPurchases(repo).Refund(context.Background(), "pu1", "changed my mind")
	if err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if p.Status != model.PurchaseStatusRefunded || p.RefundedAt == nil {
		t.Fatalf("purchase = status %q refunded_at %v, want refunded and stamped", p.Status, p.RefundedAt)
	}
	if got := repo.wallets["p1"].Coins; got != 100 {
		t.Fatalf("coins = %d, want 70+30", got)
	}
	if len(repo.txs) != 1 {
		t.Fatalf("transactions = %d, want one credit", len(repo.txs))
	}
	tx := repo.txs[0]
	if tx.Type != model.TxTypeCredit || tx.Amount != 30 || tx.Reason != "changed my mind" || tx.PurchaseID != "pu1" {
		t.Fatalf("transaction = %+v, want a credit of 30 for pu1 with the given reason", tx)
	}
}

func TestRefundAlreadyRefundedRejected(t *testing.T) {
	repo := newFakeRepo().withWallet("p1", "Bob", 70)
	p := repo.withPurchase("pu1", "p1", "Bob", 30)
	if err := p.Refund(); err != nil {
		t.Fatalf("seeding a refunded purchase: %v", err)
	}

	_, err := newPurchases(repo).Refund(context.Background(), "pu1", "again")
	if !errors.Is(err, ierror.ErrAlreadyRefunded) {
		t.Fatalf("Refund: got %v, want ErrAlreadyRefunded", err)
	}
	if got := repo.wallets["p1"].Coins; got != 70 {
		t.Fatalf("coins = %d, want no second payout", got)
	}
	if len(repo.txs) != 0 {
		t.Fatalf("transactions = %d, want none", len(repo.txs))
	}
}

func TestRefundMissingPurchaseRejected(t *testing.T) {
	repo := newFakeRepo()

	if _, err := newPurchases(repo).Refund(context.Background(), "nope", "r"); !errors.Is(err, ierror.ErrNotFound) {
		t.Fatalf("Refund: got %v, want ErrNotFound", err)
	}
}

// Documented ordering, second write: the purchase is marked before the payout,
// so a failed credit leaves a refunded purchase the player was never paid for.
// Deliberate — the alternative risks paying twice on retry.
func TestRefundLeavesThePurchaseRefundedWhenTheCreditFails(t *testing.T) {
	repo := newFakeRepo().withWallet("p1", "Bob", 70)
	repo.withPurchase("pu1", "p1", "Bob", 30)
	repo.addCoinsErr = errors.New("upsert failed")

	_, err := newPurchases(repo).Refund(context.Background(), "pu1", "r")
	if err == nil {
		t.Fatal("Refund: got nil error, want the credit failure")
	}
	if got := repo.purchases["pu1"].Status; got != model.PurchaseStatusRefunded {
		t.Fatalf("status = %q, want refunded — the mark is not rolled back", got)
	}
	if got := repo.wallets["p1"].Coins; got != 70 {
		t.Fatalf("coins = %d, want the player unpaid", got)
	}
	if len(repo.txs) != 0 {
		t.Fatalf("transactions = %d, want none", len(repo.txs))
	}

	// And a retry is refused rather than paying twice.
	repo.addCoinsErr = nil
	if _, err := newPurchases(repo).Refund(context.Background(), "pu1", "r"); !errors.Is(err, ierror.ErrAlreadyRefunded) {
		t.Fatalf("retry: got %v, want ErrAlreadyRefunded", err)
	}
	if got := repo.wallets["p1"].Coins; got != 70 {
		t.Fatalf("coins after retry = %d, want no double payout", got)
	}
}

// Documented ordering, third write: coins are already back, only the ledger row
// is missing.
func TestRefundKeepsTheCoinsWhenTheLedgerFails(t *testing.T) {
	repo := newFakeRepo().withWallet("p1", "Bob", 70)
	repo.withPurchase("pu1", "p1", "Bob", 30)
	repo.createTxErr = errors.New("insert failed")

	if _, err := newPurchases(repo).Refund(context.Background(), "pu1", "r"); err == nil {
		t.Fatal("Refund: got nil error, want the ledger failure")
	}
	if got := repo.wallets["p1"].Coins; got != 100 {
		t.Fatalf("coins = %d, want the payout to survive a ledger failure", got)
	}
}
