package donateuc

import (
	"context"

	repository "github.com/lasthearth/vsservice/internal/donate/internal/repository/mongo"
	"go.uber.org/fx"
)

var _ WalletRepo = (*repository.Repository)(nil)

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
	_, err := uc.repo.AddCoinsToWallet(ctx, playerID, playerName, amount)
	if err != nil {
		return err
	}

	return nil
}
