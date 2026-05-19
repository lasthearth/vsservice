package repository

import (
	"context"
	"time"

	dto "github.com/lasthearth/vsservice/internal/donate/internal/dto/mongo"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
	"github.com/lasthearth/vsservice/internal/donate/internal/service"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	mgo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/fx"
)

const (
	walletCollName      = "donate_wallets"
	shopItemCollName    = "donate_shop_items"
	purchaseCollName    = "donate_purchases"
	transactionCollName = "donate_transactions"
)

var _ service.DonateRepository = (*Repository)(nil)

type Repository struct {
	log        logger.Logger
	client     *mgo.Client
	walletColl *mgo.Collection
	shopColl   *mgo.Collection
	purchColl  *mgo.Collection
	txColl     *mgo.Collection
}

type Opts struct {
	fx.In

	Log      logger.Logger
	Database *mgo.Database
	Client   *mgo.Client
}

func New(opts Opts) *Repository {
	log := opts.Log.WithComponent("donate-repository")
	walletColl := opts.Database.Collection(walletCollName)
	shopColl := opts.Database.Collection(shopItemCollName)
	purchColl := opts.Database.Collection(purchaseCollName)
	txColl := opts.Database.Collection(transactionCollName)
	setupIndexes(walletColl, purchColl, txColl)
	return &Repository{
		log:        log,
		client:     opts.Client,
		walletColl: walletColl,
		shopColl:   shopColl,
		purchColl:  purchColl,
		txColl:     txColl,
	}
}

func setupIndexes(walletColl, purchColl, txColl *mgo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	walletColl.Indexes().CreateOne(ctx, mgo.IndexModel{
		Keys:    bson.D{{Key: "player_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	purchColl.Indexes().CreateOne(ctx, mgo.IndexModel{
		Keys: bson.D{{Key: "player_id", Value: 1}},
	})
	txColl.Indexes().CreateOne(ctx, mgo.IndexModel{
		Keys: bson.D{{Key: "player_id", Value: 1}},
	})
}

func walletFromDTO(d dto.Wallet) *model.Wallet {
	return &model.Wallet{
		Id:         d.Model.Id.Hex(),
		PlayerID:   d.PlayerID,
		PlayerName: d.PlayerName,
		Coins:      d.Coins,
		CreatedAt:  d.Model.CreatedAt,
		UpdatedAt:  d.Model.UpdatedAt,
	}
}

func shopItemFromDTO(d dto.ShopItem) *model.ShopItem {
	return &model.ShopItem{
		Id:          d.Model.Id.Hex(),
		Name:        d.Name,
		Description: d.Description,
		ImageURL:    d.ImageURL,
		Price:       d.Price,
		IsAvailable: d.IsAvailable,
		CreatedAt:   d.Model.CreatedAt,
		UpdatedAt:   d.Model.UpdatedAt,
	}
}

func shopItemToDTO(m *model.ShopItem) dto.ShopItem {
	d := dto.ShopItem{
		Name:        m.Name,
		Description: m.Description,
		ImageURL:    m.ImageURL,
		Price:       m.Price,
		IsAvailable: m.IsAvailable,
	}
	if m.Id != "" {
		if oid, err := mongox.ParseObjectID(m.Id); err == nil {
			d.Model.Id = oid
		}
	}
	d.Model.CreatedAt = m.CreatedAt
	d.Model.UpdatedAt = m.UpdatedAt
	return d
}

func purchaseFromDTO(d dto.Purchase) *model.Purchase {
	return &model.Purchase{
		Id:         d.Model.Id.Hex(),
		PlayerID:   d.PlayerID,
		PlayerName: d.PlayerName,
		ItemID:     d.ItemID,
		ItemName:   d.ItemName,
		PricePaid:  d.PricePaid,
		Status:     model.PurchaseStatus(d.Status),
		CreatedAt:  d.Model.CreatedAt,
		RefundedAt: d.RefundedAt,
	}
}

func txFromDTO(d dto.Transaction) *model.Transaction {
	return &model.Transaction{
		Id:         d.Model.Id.Hex(),
		PlayerID:   d.PlayerID,
		Amount:     d.Amount,
		Type:       model.TxType(d.Type),
		Reason:     d.Reason,
		PurchaseID: d.PurchaseID,
		CreatedAt:  d.Model.CreatedAt,
	}
}
