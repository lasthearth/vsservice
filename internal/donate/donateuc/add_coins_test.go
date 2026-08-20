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
type fakeWalletRepo struct {
	coins map[string]int64
	names map[string]string
	txs   []creditTx

	addErr error
	txErr  error
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

func (f *fakeWalletRepo) AddCoinsToWallet(_ context.Context, playerID, playerName string, amount int64) (int64, error) {
	if f.addErr != nil {
		return 0, f.addErr
	}
	f.coins[playerID] += amount
	if playerName != "" || f.names[playerID] == "" {
		f.names[playerID] = playerName
	}
	return f.coins[playerID], nil
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
