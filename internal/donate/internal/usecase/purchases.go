package usecase

import (
	"context"

	"github.com/lasthearth/vsservice/internal/donate/internal/model"
	"go.uber.org/fx"
)

// PurchaseRepo is the persistence port the purchase rules need. It is the
// consumer-side interface for donate's Mongo repository: domain models in,
// domain models out, no DTO or driver types.
type PurchaseRepo interface {
	GetShopItem(ctx context.Context, id string) (*model.ShopItem, error)
	GetWalletByPlayerID(ctx context.Context, playerID string) (*model.Wallet, error)
	UpdateWallet(
		ctx context.Context,
		playerID string,
		updateFn func(ctx context.Context, wallet *model.Wallet) (*model.Wallet, error),
	) error
	AddCoinsToWallet(ctx context.Context, playerID, playerName string, amount int64) (int64, error)
	CreatePurchase(ctx context.Context, purchase *model.Purchase) (*model.Purchase, error)
	UpdatePurchase(
		ctx context.Context,
		id string,
		updateFn func(ctx context.Context, p *model.Purchase) (*model.Purchase, error),
	) (*model.Purchase, error)
	CreateTransaction(ctx context.Context, tx *model.Transaction) (*model.Transaction, error)
}

type Opts struct {
	fx.In

	Repo PurchaseRepo
	Seq  Sequence
}

// Purchases owns the rules that move coins in or out of a wallet against a
// purchase record. They share one port and one Sequence, so they share a struct.
type Purchases struct {
	repo PurchaseRepo
	seq  Sequence
}

func NewPurchases(opts Opts) *Purchases {
	return &Purchases{repo: opts.Repo, seq: opts.Seq}
}
