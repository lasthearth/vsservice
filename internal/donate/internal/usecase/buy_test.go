package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/donate/internal/ierror"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
)

func TestBuyAtFullPrice(t *testing.T) {
	repo := newFakeRepo().withWallet("p1", "Bob", 100)
	repo.withItem("i1", "Sword", 30)

	p, err := newPurchases(repo).Buy(context.Background(), "p1", "i1")
	if err != nil {
		t.Fatalf("Buy: %v", err)
	}

	if p.PricePaid != 30 || p.BasePrice != 30 || p.DiscountPercent != 0 {
		t.Fatalf("purchase = paid %d base %d discount %d, want 30/30/0", p.PricePaid, p.BasePrice, p.DiscountPercent)
	}
	if p.PlayerName != "Bob" {
		t.Fatalf("player_name = %q, want the name stored on the wallet", p.PlayerName)
	}
	if p.Status != model.PurchaseStatusActive {
		t.Fatalf("status = %q, want active", p.Status)
	}
	if got := repo.wallets["p1"].Coins; got != 70 {
		t.Fatalf("coins = %d, want 70", got)
	}
	if len(repo.txs) != 1 {
		t.Fatalf("transactions = %d, want exactly one debit", len(repo.txs))
	}
	tx := repo.txs[0]
	if tx.Type != model.TxTypeDebit || tx.Amount != 30 || tx.PurchaseID != p.Id {
		t.Fatalf("transaction = %+v, want a debit of 30 attached to %s", tx, p.Id)
	}
}

func TestBuyInsideDiscountWindowCapturesTheDiscount(t *testing.T) {
	repo := newFakeRepo().withWallet("p1", "Bob", 100)
	item := repo.withItem("i1", "Sword", 50)
	if err := item.SetDiscount(40); err != nil {
		t.Fatalf("SetDiscount: %v", err)
	}
	start := time.Now().Add(-time.Hour)
	end := time.Now().Add(time.Hour)
	item.SetDiscountWindow(&start, &end)

	p, err := newPurchases(repo).Buy(context.Background(), "p1", "i1")
	if err != nil {
		t.Fatalf("Buy: %v", err)
	}

	// 40% off 50 is 30, and the purchase must remember what it was discounted
	// from so a later price change cannot rewrite history.
	if p.PricePaid != 30 {
		t.Fatalf("price_paid = %d, want 30", p.PricePaid)
	}
	if p.BasePrice != 50 {
		t.Fatalf("base_price = %d, want the undiscounted 50", p.BasePrice)
	}
	if p.DiscountPercent != 40 {
		t.Fatalf("discount_percent = %d, want 40", p.DiscountPercent)
	}
	if got := repo.wallets["p1"].Coins; got != 70 {
		t.Fatalf("coins = %d, want 100-30", got)
	}
	if repo.txs[0].Amount != 30 {
		t.Fatalf("ledger amount = %d, want the discounted 30", repo.txs[0].Amount)
	}
}

// A discount whose window has closed must not apply, even with HasDiscount set.
func TestBuyOutsideDiscountWindowPaysFullPrice(t *testing.T) {
	repo := newFakeRepo().withWallet("p1", "Bob", 100)
	item := repo.withItem("i1", "Sword", 50)
	if err := item.SetDiscount(40); err != nil {
		t.Fatalf("SetDiscount: %v", err)
	}
	start := time.Now().Add(-2 * time.Hour)
	end := time.Now().Add(-time.Hour)
	item.SetDiscountWindow(&start, &end)

	p, err := newPurchases(repo).Buy(context.Background(), "p1", "i1")
	if err != nil {
		t.Fatalf("Buy: %v", err)
	}
	if p.PricePaid != 50 || p.DiscountPercent != 0 {
		t.Fatalf("purchase = paid %d discount %d, want 50/0", p.PricePaid, p.DiscountPercent)
	}
}

func TestBuyWithInsufficientBalanceWritesNothing(t *testing.T) {
	repo := newFakeRepo().withWallet("p1", "Bob", 10)
	repo.withItem("i1", "Sword", 30)

	_, err := newPurchases(repo).Buy(context.Background(), "p1", "i1")
	if !errors.Is(err, ierror.ErrInsufficientFunds) {
		t.Fatalf("Buy: got %v, want ErrInsufficientFunds", err)
	}
	if got := repo.wallets["p1"].Coins; got != 10 {
		t.Fatalf("coins = %d, want the balance untouched", got)
	}
	if len(repo.purchases) != 0 || len(repo.txs) != 0 {
		t.Fatalf("wrote purchases=%d txs=%d, want none", len(repo.purchases), len(repo.txs))
	}
}

func TestBuyUnavailableItemRejected(t *testing.T) {
	repo := newFakeRepo().withWallet("p1", "Bob", 100)
	item := repo.withItem("i1", "Sword", 30)
	item.Apply(model.ShopItemUpdate{
		Code: item.Code, Name: item.Name, Price: item.Price, Type: item.Type, IsAvailable: false,
	})

	_, err := newPurchases(repo).Buy(context.Background(), "p1", "i1")
	if !errors.Is(err, ierror.ErrNotFound) {
		t.Fatalf("Buy: got %v, want ErrNotFound", err)
	}
	if repo.walletCalls != 0 {
		t.Fatalf("wallet touched %d times, want 0 — availability is decided first", repo.walletCalls)
	}
}

func TestBuyMissingItemRejected(t *testing.T) {
	repo := newFakeRepo().withWallet("p1", "Bob", 100)

	if _, err := newPurchases(repo).Buy(context.Background(), "p1", "nope"); !errors.Is(err, ierror.ErrNotFound) {
		t.Fatalf("Buy: got %v, want ErrNotFound", err)
	}
}

// Documented ordering, second write: the wallet is debited before the purchase
// record exists, so a failed CreatePurchase must give the coins back.
func TestBuyReturnsCoinsWhenThePurchaseRecordFails(t *testing.T) {
	repo := newFakeRepo().withWallet("p1", "Bob", 100)
	repo.withItem("i1", "Sword", 30)
	repo.createPurErr = errors.New("insert failed")

	_, err := newPurchases(repo).Buy(context.Background(), "p1", "i1")
	if err == nil {
		t.Fatal("Buy: got nil error, want the insert failure")
	}
	if got := repo.wallets["p1"].Coins; got != 100 {
		t.Fatalf("coins = %d, want 100 — the withdrawal must be compensated", got)
	}
	if len(repo.purchases) != 0 || len(repo.txs) != 0 {
		t.Fatalf("wrote purchases=%d txs=%d, want none", len(repo.purchases), len(repo.txs))
	}
}

// If the compensating credit also fails the coins really are lost, so the error
// must name both failures rather than hide one.
func TestBuyReportsBothFailuresWhenCompensationFails(t *testing.T) {
	repo := newFakeRepo().withWallet("p1", "Bob", 100)
	repo.withItem("i1", "Sword", 30)
	repo.createPurErr = errors.New("insert failed")
	repo.addCoinsErr = errors.New("upsert failed")

	_, err := newPurchases(repo).Buy(context.Background(), "p1", "i1")
	if err == nil {
		t.Fatal("Buy: got nil error, want both failures")
	}
	msg := err.Error()
	if !strings.Contains(msg, "insert failed") || !strings.Contains(msg, "upsert failed") {
		t.Fatalf("error = %q, want it to name both the insert and the failed compensation", msg)
	}
	if got := repo.wallets["p1"].Coins; got != 70 {
		t.Fatalf("coins = %d, want the debited 70 — the compensation failed", got)
	}
}

// Documented ordering, third write: a failed ledger entry leaves a paid-for
// purchase in place. The player keeps what they bought; only the report is short.
func TestBuyKeepsThePurchaseWhenTheLedgerFails(t *testing.T) {
	repo := newFakeRepo().withWallet("p1", "Bob", 100)
	repo.withItem("i1", "Sword", 30)
	repo.createTxErr = errors.New("insert failed")

	_, err := newPurchases(repo).Buy(context.Background(), "p1", "i1")
	if err == nil {
		t.Fatal("Buy: got nil error, want the ledger failure")
	}
	if len(repo.purchases) != 1 {
		t.Fatalf("purchases = %d, want the purchase to survive a ledger failure", len(repo.purchases))
	}
	if got := repo.wallets["p1"].Coins; got != 70 {
		t.Fatalf("coins = %d, want the player charged for the purchase they got", got)
	}
	if len(repo.txs) != 0 {
		t.Fatalf("transactions = %d, want none", len(repo.txs))
	}
}
