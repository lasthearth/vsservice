package donateuc_test

import (
	"context"
	"errors"
	"testing"

	"github.com/lasthearth/vsservice/internal/donate/donateuc"
)

// fakeWalletRepo is a hand-written stand-in for donateuc.WalletRepo. Its
// AddCoinsToWallet reproduces the contract of donate's Mongo implementation:
// an empty playerName never overwrites a name already stored on the wallet.
// Its wallet writes respect ctx cancellation, so a compensation that runs on
// a dead request context fails the way the real one would.
type fakeWalletRepo struct {
	coins map[string]int64
	names map[string]string
	txs   []creditTx
	debts []debitTx

	addErr     error
	txErr      error
	debitTxErr error
}

type debitTx struct {
	playerID string
	amount   int64
	reason   string
}

type creditTx struct {
	playerID string
	amount   int64
	reason   string
}

func newFakeWalletRepo() *fakeWalletRepo {
	return &fakeWalletRepo{
		coins: map[string]int64{},
		names: map[string]string{},
	}
}

func (f *fakeWalletRepo) AddCoinsToWallet(ctx context.Context, playerID, playerName string, amount int64) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if f.addErr != nil {
		return 0, f.addErr
	}
	f.coins[playerID] += amount
	if playerName != "" || f.names[playerID] == "" {
		f.names[playerID] = playerName
	}
	return f.coins[playerID], nil
}

func (f *fakeWalletRepo) WithdrawCoins(_ context.Context, playerID string, amount int64) error {
	if f.coins[playerID] < amount {
		return donateuc.ErrInsufficientFunds
	}
	f.coins[playerID] -= amount
	return nil
}

func (f *fakeWalletRepo) CreateDebitTransaction(ctx context.Context, playerID string, amount int64, reason string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.debitTxErr != nil {
		return f.debitTxErr
	}
	f.debts = append(f.debts, debitTx{playerID: playerID, amount: amount, reason: reason})
	return nil
}

func (f *fakeWalletRepo) CreateCreditTransaction(_ context.Context, playerID string, amount int64, reason string) error {
	if f.txErr != nil {
		return f.txErr
	}
	f.txs = append(f.txs, creditTx{playerID: playerID, amount: amount, reason: reason})
	return nil
}

func newUC(repo donateuc.WalletRepo) *donateuc.AddCoinsUseCase {
	return donateuc.NewAddCoinsUseCase(donateuc.Opts{Repo: repo})
}

func TestNonPositiveAmountRejected(t *testing.T) {
	for _, amount := range []int64{0, -1, -100} {
		repo := newFakeWalletRepo()
		uc := newUC(repo)

		if err := uc.AddCoins(context.Background(), "p1", "Bob", amount); !errors.Is(err, donateuc.ErrNonPositiveAmount) {
			t.Fatalf("AddCoins(%d): got %v, want ErrNonPositiveAmount", amount, err)
		}
		if err := uc.Credit(context.Background(), "p1", "Bob", amount, "reason"); !errors.Is(err, donateuc.ErrNonPositiveAmount) {
			t.Fatalf("Credit(%d): got %v, want ErrNonPositiveAmount", amount, err)
		}
		if len(repo.coins) != 0 || len(repo.txs) != 0 {
			t.Fatalf("amount %d reached the repository: coins=%v txs=%v", amount, repo.coins, repo.txs)
		}
	}
}

// A credit with no display name must not blank a name stored by an earlier
// credit. This was the live bug in hungergames' hand-rolled copy of the wallet
// upsert, which $set player_name unconditionally.
func TestEmptyPlayerNameDoesNotBlankStoredName(t *testing.T) {
	repo := newFakeWalletRepo()
	uc := newUC(repo)

	if err := uc.AddCoins(context.Background(), "p1", "Bob", 10); err != nil {
		t.Fatalf("first credit: %v", err)
	}
	if err := uc.Credit(context.Background(), "p1", "", 5, "Season 3 reward, rank 1"); err != nil {
		t.Fatalf("second credit: %v", err)
	}

	if got := repo.names["p1"]; got != "Bob" {
		t.Fatalf("player_name = %q, want %q", got, "Bob")
	}
	if got := repo.coins["p1"]; got != 15 {
		t.Fatalf("coins = %d, want 15", got)
	}
	if len(repo.txs) != 1 || repo.txs[0] != (creditTx{playerID: "p1", amount: 5, reason: "Season 3 reward, rank 1"}) {
		t.Fatalf("transactions = %+v, want one credit of 5", repo.txs)
	}
}

func TestCreditRecordsLedgerEntry(t *testing.T) {
	repo := newFakeWalletRepo()

	if err := newUC(repo).Credit(context.Background(), "p1", "Bob", 7, "reward"); err != nil {
		t.Fatalf("Credit: %v", err)
	}
	if got := repo.coins["p1"]; got != 7 {
		t.Fatalf("coins = %d, want 7", got)
	}
	if len(repo.txs) != 1 {
		t.Fatalf("transactions = %+v, want exactly one", repo.txs)
	}
}

func TestCreditKeepsCoinsWhenLedgerFails(t *testing.T) {
	repo := newFakeWalletRepo()
	repo.txErr = errors.New("insert failed")

	err := newUC(repo).Credit(context.Background(), "p1", "Bob", 7, "reward")
	if err == nil {
		t.Fatal("Credit: got nil error, want the ledger error")
	}
	if got := repo.coins["p1"]; got != 7 {
		t.Fatalf("coins = %d, want the wallet increment to survive a ledger failure", got)
	}
}

func TestCreditSkipsLedgerWhenWalletFails(t *testing.T) {
	repo := newFakeWalletRepo()
	repo.addErr = errors.New("upsert failed")

	if err := newUC(repo).Credit(context.Background(), "p1", "Bob", 7, "reward"); err == nil {
		t.Fatal("Credit: got nil error, want the wallet error")
	}
	if len(repo.txs) != 0 {
		t.Fatalf("transactions = %+v, want none when the wallet write failed", repo.txs)
	}
}

func TestDebitWithdrawsAndRecords(t *testing.T) {
	repo := newFakeWalletRepo()
	repo.coins["p1"] = 5000
	uc := donateuc.NewAddCoinsUseCase(donateuc.Opts{Repo: repo})

	if err := uc.Debit(context.Background(), "p1", 3000, "appearance: banner piper"); err != nil {
		t.Fatal(err)
	}
	if repo.coins["p1"] != 2000 {
		t.Fatalf("balance = %d, want 2000", repo.coins["p1"])
	}
	if len(repo.debts) != 1 || repo.debts[0].amount != 3000 || repo.debts[0].reason != "appearance: banner piper" {
		t.Fatalf("ledger = %+v", repo.debts)
	}
}

func TestDebitWithoutEnoughCoinsTakesNothing(t *testing.T) {
	repo := newFakeWalletRepo()
	repo.coins["p1"] = 100
	uc := donateuc.NewAddCoinsUseCase(donateuc.Opts{Repo: repo})

	if err := uc.Debit(context.Background(), "p1", 3000, "x"); !errors.Is(err, donateuc.ErrInsufficientFunds) {
		t.Fatalf("err = %v, want ErrInsufficientFunds", err)
	}
	if repo.coins["p1"] != 100 || len(repo.debts) != 0 {
		t.Fatalf("something changed: coins=%d debts=%v", repo.coins["p1"], repo.debts)
	}
}

func TestDebitRejectsNonPositiveAmount(t *testing.T) {
	uc := donateuc.NewAddCoinsUseCase(donateuc.Opts{Repo: newFakeWalletRepo()})
	if err := uc.Debit(context.Background(), "p1", 0, "x"); !errors.Is(err, donateuc.ErrNonPositiveAmount) {
		t.Fatalf("err = %v, want ErrNonPositiveAmount", err)
	}
}

// A failed ledger write returns the coins: the caller aborts its purchase, so
// nothing was granted for the withdrawal.
func TestDebitRefundsWhenLedgerFails(t *testing.T) {
	repo := newFakeWalletRepo()
	repo.coins["p1"] = 5000
	repo.debitTxErr = errors.New("insert failed")
	uc := donateuc.NewAddCoinsUseCase(donateuc.Opts{Repo: repo})

	err := uc.Debit(context.Background(), "p1", 3000, "appearance: banner piper")
	if err == nil || !errors.Is(err, repo.debitTxErr) {
		t.Fatalf("err = %v, want the ledger error", err)
	}
	if repo.coins["p1"] != 5000 {
		t.Fatalf("balance = %d, want the withdrawal returned", repo.coins["p1"])
	}
	if len(repo.debts) != 0 {
		t.Fatalf("ledger = %+v, want no debit row", repo.debts)
	}
}

// The refund must survive a dead request context: the failure that stopped
// the ledger write (the client hanging up among them) is exactly the state in
// which the coins must still go back.
func TestDebitRefundRunsOnADetachedContext(t *testing.T) {
	repo := newFakeWalletRepo()
	repo.coins["p1"] = 5000
	uc := donateuc.NewAddCoinsUseCase(donateuc.Opts{Repo: repo})

	ctx, cancel := context.WithCancel(context.Background())
	// The withdrawal commits, then the request dies mid-Debit: every wallet
	// write from here on fails on this context, including a naive refund.
	cancel()

	if err := uc.Debit(ctx, "p1", 3000, "x"); err == nil {
		t.Fatal("Debit: want the ledger error")
	}
	if repo.coins["p1"] != 5000 {
		t.Fatalf("balance = %d, want the withdrawal returned despite the canceled request", repo.coins["p1"])
	}
}

// When the refund fails too, the joined error names both failures and the
// coins stay withdrawn — a human has to sort it out.
func TestDebitNamesBothFailuresWhenRefundFails(t *testing.T) {
	repo := newFakeWalletRepo()
	repo.coins["p1"] = 5000
	repo.debitTxErr = errors.New("insert failed")
	repo.addErr = errors.New("refund upsert failed")
	uc := donateuc.NewAddCoinsUseCase(donateuc.Opts{Repo: repo})

	err := uc.Debit(context.Background(), "p1", 3000, "x")
	if err == nil {
		t.Fatal("Debit: want the joined error")
	}
	if !errors.Is(err, repo.debitTxErr) || !errors.Is(err, repo.addErr) {
		t.Fatalf("err = %v, want both the ledger and the refund failure wrapped", err)
	}
	if repo.coins["p1"] != 2000 {
		t.Fatalf("balance = %d, want the coins to stay withdrawn", repo.coins["p1"])
	}
}
