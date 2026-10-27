package service_test

import (
	"context"
	"testing"
	"time"

	donatev1 "github.com/lasthearth/vsservice/gen/donate/v1"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
	"github.com/lasthearth/vsservice/internal/donate/internal/service"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// opRepo fakes the wallet slice of DonateRepository. The embedded interface is
// nil, so any other call panics and says so.
type opRepo struct {
	service.DonateRepository
	wallet *model.Wallet
}

func (r *opRepo) AddCoinsToWallet(ctx context.Context, playerID, playerName string, amount int64) (int64, error) {
	if r.wallet == nil {
		r.wallet = model.NewWallet(playerID, playerName)
	}
	// Deposit keeps the model's invariant check while the fake answers the new
	// balance the real $inc would.
	if err := r.wallet.Deposit(amount); err != nil {
		return 0, err
	}
	return r.wallet.Coins, nil
}

func (r *opRepo) UpdateWallet(ctx context.Context, _ string, updateFn func(context.Context, *model.Wallet) (*model.Wallet, error)) error {
	w, err := updateFn(ctx, r.wallet)
	if err != nil {
		return err
	}
	r.wallet = w
	return nil
}

func (r *opRepo) CreateTransaction(ctx context.Context, tx *model.Transaction) (*model.Transaction, error) {
	return tx, nil
}

// newOpService builds the service over an observer core, so the test sees the
// records the operations emit and nothing else.
func newOpService(t *testing.T, repo service.DonateRepository) (*service.Service, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.InfoLevel)
	return service.New(service.Opts{Repo: repo, Logger: logger.NewWithCore(core)}), logs
}

// opString returns one record's string field, and fails the test when the field
// is absent: a missing assertion target is a broken record, not a pass.
func opString(t *testing.T, e observer.LoggedEntry, key string) string {
	t.Helper()
	for _, f := range e.Context {
		if f.Key == key && f.Type == zapcore.StringType {
			return f.String
		}
	}
	t.Fatalf("record %q carries no string field %q", e.Message, key)
	return ""
}

// opInt64 returns one record's int64 field, and fails the test when the field
// is absent.
func opInt64(t *testing.T, e observer.LoggedEntry, key string) int64 {
	t.Helper()
	for _, f := range e.Context {
		if f.Key == key && f.Type == zapcore.Int64Type {
			return f.Integer
		}
	}
	t.Fatalf("record %q carries no int64 field %q", e.Message, key)
	return 0
}

// opFieldPresent reports whether a key is on the record at all, whatever its
// type.
func opFieldPresent(e observer.LoggedEntry, key string) bool {
	for _, f := range e.Context {
		if f.Key == key {
			return true
		}
	}
	return false
}

// A coin grant writes exactly one operation record: the event key, the entity,
// the actor and the new balance all on it, and no second line beside it.
func TestAddCoinsEmitsOneWalletCreditRecord(t *testing.T) {
	repo := &opRepo{}
	svc, logs := newOpService(t, repo)

	ctx := interceptor.ContextWithUserID(context.Background(), "admin1")
	if _, err := svc.AddCoins(ctx, &donatev1.AddCoinsRequest{
		PlayerId:   "p1",
		PlayerName: "Player One",
		Amount:     50,
		Reason:     "referral reward",
	}); err != nil {
		t.Fatal(err)
	}

	if logs.Len() != 1 {
		t.Fatalf("records = %d, want exactly the wallet credit record: %v", logs.Len(), logs.All())
	}
	e := logs.All()[0]
	if e.Level != zapcore.InfoLevel {
		t.Fatalf("level = %s, want Info", e.Level)
	}
	if got := opString(t, e, "event"); got != "donate.wallet.credited" {
		t.Fatalf("event = %q, want donate.wallet.credited", got)
	}
	if got := opString(t, e, "component"); got != "op" {
		t.Fatalf("component = %q, want op", got)
	}
	if got := opString(t, e, "entity"); got != "wallet" {
		t.Fatalf("entity = %q, want wallet", got)
	}
	if got := opString(t, e, "entity_id"); got != "p1" {
		t.Fatalf("entity_id = %q, want p1", got)
	}
	if got := opString(t, e, "actor_id"); got != "admin1" {
		t.Fatalf("actor_id = %q, want admin1", got)
	}
	if got := opString(t, e, "reason"); got != "referral reward" {
		t.Fatalf("reason = %q, want the grant reason", got)
	}
	if got := opInt64(t, e, "amount"); got != 50 {
		t.Fatalf("amount = %d, want 50", got)
	}
	if got := opInt64(t, e, "balance"); got != 50 {
		t.Fatalf("balance = %d, want 50", got)
	}
	if got := opString(t, e, "currency"); got != "coins" {
		t.Fatalf("currency = %q, want coins", got)
	}
	// The correlation the interceptor put in ctx must ride on the record too.
	if got := opString(t, e, "user_id"); got != "admin1" {
		t.Fatalf("user_id = %q, want admin1", got)
	}
}

// A successful deduction states the balance it left behind, taken from the
// wallet the update closure already read.
func TestDeductCoinsEmitsOneWalletDebitRecord(t *testing.T) {
	repo := &opRepo{wallet: model.ReconstituteWallet("w1", "p1", "Player One", 100, time.Time{}, time.Time{})}
	svc, logs := newOpService(t, repo)

	ctx := interceptor.ContextWithUserID(context.Background(), "admin1")
	if _, err := svc.DeductCoins(ctx, &donatev1.DeductCoinsRequest{
		PlayerId: "p1",
		Amount:   40,
		Reason:   "admin correction",
	}); err != nil {
		t.Fatal(err)
	}

	if logs.Len() != 1 {
		t.Fatalf("records = %d, want exactly the wallet debit record: %v", logs.Len(), logs.All())
	}
	e := logs.All()[0]
	if e.Level != zapcore.InfoLevel {
		t.Fatalf("level = %s, want Info", e.Level)
	}
	if got := opString(t, e, "event"); got != "donate.wallet.debited" {
		t.Fatalf("event = %q, want donate.wallet.debited", got)
	}
	if got := opString(t, e, "entity_id"); got != "p1" {
		t.Fatalf("entity_id = %q, want p1", got)
	}
	if got := opInt64(t, e, "amount"); got != 40 {
		t.Fatalf("amount = %d, want 40", got)
	}
	if got := opInt64(t, e, "balance"); got != 60 {
		t.Fatalf("balance = %d, want 60", got)
	}
}

// Insufficient funds is a rule the player can act on, not a failure: the
// record is a Warn with the reason, and nothing was debited.
func TestDeductCoinsEmitsDebitRefusedAtWarn(t *testing.T) {
	repo := &opRepo{wallet: model.ReconstituteWallet("w1", "p1", "Player One", 10, time.Time{}, time.Time{})}
	svc, logs := newOpService(t, repo)

	ctx := interceptor.ContextWithUserID(context.Background(), "admin1")
	if _, err := svc.DeductCoins(ctx, &donatev1.DeductCoinsRequest{PlayerId: "p1", Amount: 40}); err == nil {
		t.Fatal("DeductCoins: got nil error, want FailedPrecondition")
	}

	if logs.Len() != 1 {
		t.Fatalf("records = %d, want exactly the refusal record: %v", logs.Len(), logs.All())
	}
	e := logs.All()[0]
	if e.Level != zapcore.WarnLevel {
		t.Fatalf("level = %s, want Warn", e.Level)
	}
	if got := opString(t, e, "event"); got != "donate.wallet.debit.refused" {
		t.Fatalf("event = %q, want donate.wallet.debit.refused", got)
	}
	if got := opString(t, e, "entity_id"); got != "p1" {
		t.Fatalf("entity_id = %q, want p1", got)
	}
	if got := opString(t, e, "reason"); got != "insufficient funds" {
		t.Fatalf("reason = %q, want insufficient funds", got)
	}
	if opFieldPresent(e, "balance") {
		t.Fatal("balance on a refused debit: the wallet was not touched")
	}
}
