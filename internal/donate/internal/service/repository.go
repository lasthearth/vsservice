package service

import (
	"context"
	"io"

	"github.com/lasthearth/vsservice/internal/donate/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/storage"
	"github.com/minio/minio-go/v7"
)

// Storage is the subset of pkg/storage.Storage used by the donate service.
var _ Storage = (*storage.Storage)(nil)

type Storage interface {
	BucketExists(ctx context.Context, bucketName string) (bool, error)
	MakeBucketPublic(ctx context.Context, bucketName string) error
	CreateBucket(ctx context.Context, bucketName string) error
	UploadObject(
		ctx context.Context,
		bucketName, objectName string,
		reader io.Reader,
		size int64,
		contentType string,
	) (*minio.UploadInfo, error)
}

// DonateRepository is the single persistence interface for the donate domain.
// All repository methods are defined here; the implementation lives in
// internal/donate/internal/repository/mongo.
type DonateRepository interface {
	// Wallet

	// GetWalletByPlayerID returns the wallet for playerID.
	// Returns ierror.ErrNotFound if the player has no wallet yet.
	GetWalletByPlayerID(ctx context.Context, playerID string) (*model.Wallet, error)

	// AddCoinsToWallet atomically upserts the wallet and increments coins by amount.
	// Creates the wallet if it does not exist.
	AddCoinsToWallet(ctx context.Context, playerID, playerName string, amount int64) (newCoins int64, err error)

	// UpdateWallet reads the wallet then applies updateFn and saves the result.
	// Returns ierror.ErrNotFound if the wallet does not exist.
	UpdateWallet(
		ctx context.Context,
		playerID string,
		updateFn func(ctx context.Context, wallet *model.Wallet) (*model.Wallet, error),
	) error

	// Shop items

	CreateShopItem(ctx context.Context, item *model.ShopItem) (*model.ShopItem, error)
	GetShopItem(ctx context.Context, id string) (*model.ShopItem, error)
	UpdateShopItem(
		ctx context.Context,
		id string,
		updateFn func(ctx context.Context, item *model.ShopItem) (*model.ShopItem, error),
	) (*model.ShopItem, error)
	DeleteShopItem(ctx context.Context, id string) error
	ListShopItems(ctx context.Context, availableOnly bool) ([]*model.ShopItem, error)

	// Purchases

	GetPurchase(ctx context.Context, id string) (*model.Purchase, error)
	ListPurchasesByPlayerID(ctx context.Context, playerID string) ([]*model.Purchase, error)

	// Transactions

	CreateTransaction(ctx context.Context, tx *model.Transaction) (*model.Transaction, error)
	ListTransactionsByPlayerID(ctx context.Context, playerID string) ([]*model.Transaction, error)

	// Atomic operations

	// BuyItem atomically deducts coins from the wallet, creates a purchase record,
	// and records a debit transaction — all within a single MongoDB session.
	// playerName is resolved from the wallet document (set by admin via AddCoins).
	BuyItem(ctx context.Context, playerID, itemID string) (*model.Purchase, error)

	// Refund atomically marks the purchase as refunded, restores coins to the wallet,
	// and records a credit transaction — all within a single MongoDB session.
	Refund(ctx context.Context, purchaseID, reason string) (*model.Purchase, error)
}
