package donateuc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/fx"
)

var (
	// ErrNonPositiveAmount is returned when a caller credits or debits a
	// non-positive amount.
	ErrNonPositiveAmount = errors.New("amount must be positive")
	// ErrInsufficientFunds is returned by Debit when the player has no wallet
	// or not enough coins in it. Nothing is withdrawn.
	ErrInsufficientFunds = errors.New("insufficient funds")
)

// WalletRepo is the donate-side write port used by other domains. It is
// deliberately primitive-typed so donate's internal model and DTO types never
// cross this seam. Bound to the donate Mongo repository in internal/donate/fx.go.
type WalletRepo interface {
	AddCoinsToWallet(ctx context.Context, playerID, playerName string, amount int64) (int64, error)
	CreateCreditTransaction(ctx context.Context, playerID string, amount int64, reason string) error
	// WithdrawCoins takes amount from the wallet, or returns
	// ErrInsufficientFunds and takes nothing.
	WithdrawCoins(ctx context.Context, playerID string, amount int64) error
	CreateDebitTransaction(ctx context.Context, playerID string, amount int64, reason string) error
}

type Opts struct {
	fx.In
	Repo WalletRepo
}

type AddCoinsUseCase struct {
	repo WalletRepo
}

func NewAddCoinsUseCase(opts Opts) *AddCoinsUseCase {
	return &AddCoinsUseCase{
		repo: opts.Repo,
	}
}

// AddCoins credits amount donate-coins to playerID's wallet, creating the
// wallet if it does not exist. The resulting balance is discarded; callers
// outside the donate domain only need to know whether the operation succeeded.
//
// An empty playerName means "no display name to report" and never overwrites a
// name already stored on the wallet.
func (uc *AddCoinsUseCase) AddCoins(ctx context.Context, playerID, playerName string, amount int64) error {
	if amount <= 0 {
		return ErrNonPositiveAmount
	}

	_, err := uc.repo.AddCoinsToWallet(ctx, playerID, playerName, amount)
	if err != nil {
		return err
	}

	return nil
}

// Credit adds coins to playerID's wallet and records a credit entry in donate's
// transaction ledger, so a cross-domain reward is a single call rather than two
// a caller must remember to pair. The wallet increment is the operation that
// must not be lost: if the ledger write fails the coins stay credited and the
// error is returned so the caller can log it.
func (uc *AddCoinsUseCase) Credit(ctx context.Context, playerID, playerName string, amount int64, reason string) error {
	if err := uc.AddCoins(ctx, playerID, playerName, amount); err != nil {
		return err
	}

	return uc.repo.CreateCreditTransaction(ctx, playerID, amount, reason)
}

// refundTimeout bounds a compensation write that runs after the operation
// failed: long enough for one Mongo round-trip, short enough not to pin a
// request that is already gone.
const refundTimeout = 5 * time.Second

// Debit takes amount coins from playerID's wallet for something bought outside
// the donate shop and records a debit entry in the ledger. A player without a
// wallet or without enough coins gets ErrInsufficientFunds and loses nothing.
//
// The caller aborts its own flow when Debit returns an error (the purchase it
// was paying for did not happen), so the withdrawal is compensated: if the
// ledger write fails, the coins go back. Unlike the donate shop's own Buy —
// where a failed ledger row leaves the purchase standing and the player with
// what they paid for — here nothing was granted yet. The refund runs on a
// detached context, because the failure that stopped the ledger write (a
// canceled request among them) must not stop the refund too. If the refund
// fails as well the coins are lost and the joined error names both failures —
// no self-healing, it needs a human.
func (uc *AddCoinsUseCase) Debit(ctx context.Context, playerID string, amount int64, reason string) error {
	if amount <= 0 {
		return ErrNonPositiveAmount
	}
	if err := uc.repo.WithdrawCoins(ctx, playerID, amount); err != nil {
		return err
	}

	if err := uc.repo.CreateDebitTransaction(ctx, playerID, amount, reason); err != nil {
		refundCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), refundTimeout)
		defer cancel()
		if cerr := uc.repo.AddCoinsToWallet(refundCtx, playerID, "", amount); cerr != nil {
			return fmt.Errorf(
				"debit ledger write failed (%w) and returning the %d withdrawn coins failed: %w",
				err, amount, cerr,
			)
		}
		return err
	}
	return nil
}
