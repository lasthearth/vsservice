package donateuc

import (
	"context"
	"errors"

	"go.uber.org/fx"
)

// WalletRepo is the donate-side write port used by other domains. It is
// deliberately primitive-typed so donate's internal model and DTO types never
// cross this seam. Bound to the donate Mongo repository in internal/donate/fx.go.
type WalletRepo interface {
	AddCoinsToWallet(ctx context.Context, playerID, playerName string, amount int64) (int64, error)
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
func (uc *AddCoinsUseCase) AddCoins(ctx context.Context, playerID, playerName string, amount int64) error {
	if amount <= 0 {
		return errors.New("amount must be positive")
	}

	_, err := uc.repo.AddCoinsToWallet(ctx, playerID, playerName, amount)
	if err != nil {
		return err
	}

	return nil
}
