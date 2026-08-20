package donateuc

import (
	"context"
	"errors"

	"go.uber.org/fx"
)

// ErrNonPositiveAmount is returned when a caller credits a non-positive amount.
var ErrNonPositiveAmount = errors.New("amount must be positive")

// WalletRepo is the donate-side write port used by other domains. It is
// deliberately primitive-typed so donate's internal model and DTO types never
// cross this seam. Bound to the donate Mongo repository in internal/donate/fx.go.
type WalletRepo interface {
	AddCoinsToWallet(ctx context.Context, playerID, playerName string, amount int64) (int64, error)
	CreateCreditTransaction(ctx context.Context, playerID string, amount int64, reason string) error
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
