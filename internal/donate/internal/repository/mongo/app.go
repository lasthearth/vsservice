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
	"go.uber.org/zap"
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
	setupIndexes(log, walletColl, shopColl, purchColl, txColl)
	return &Repository{
		log:        log,
		client:     opts.Client,
		walletColl: walletColl,
		shopColl:   shopColl,
		purchColl:  purchColl,
		txColl:     txColl,
	}
}

func setupIndexes(log logger.Logger, walletColl, shopColl, purchColl, txColl *mgo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	createIndex := func(coll *mgo.Collection, model mgo.IndexModel) {
		if _, err := coll.Indexes().CreateOne(ctx, model); err != nil {
			log.Error("failed to create index", zap.String("collection", coll.Name()), zap.Error(err))
		}
	}

	createIndex(walletColl, mgo.IndexModel{
		Keys:    bson.D{{Key: "player_id", Value: 1}, {Key: "player_name", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	createIndex(shopColl, mgo.IndexModel{
		Keys:    bson.D{{Key: "code", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	createIndex(purchColl, mgo.IndexModel{
		Keys: bson.D{{Key: "player_id", Value: 1}},
	})
	createIndex(purchColl, mgo.IndexModel{
		Keys: bson.D{{Key: "status", Value: 1}, {Key: "_id", Value: -1}},
		Options: options.Index().SetPartialFilterExpression(bson.M{
			"issued_at": bson.M{"$exists": false},
		}),
	})
	createIndex(txColl, mgo.IndexModel{
		Keys: bson.D{{Key: "player_id", Value: 1}},
	})
}

func walletFromDTO(d dto.Wallet) *model.Wallet {
	return &model.Wallet{
		Id:         d.Id.Hex(),
		PlayerID:   d.PlayerID,
		PlayerName: d.PlayerName,
		Coins:      d.Coins,
		CreatedAt:  d.CreatedAt,
		UpdatedAt:  d.UpdatedAt,
	}
}

func shopItemFromDTO(d dto.ShopItem) *model.ShopItem {
	t := model.ItemType(d.Type)
	if t == "" {
		t = model.ItemTypeItem
	}

	entries := make([]model.KitEntry, len(d.Entries))
	for i, e := range d.Entries {
		entries[i] = model.KitEntry{
			Name:        e.Name,
			Description: e.Description,
			ImageURL:    e.ImageURL,
			Quantity:    e.Quantity,
		}
	}

	return &model.ShopItem{
		Id:              d.Id.Hex(),
		Code:            d.Code,
		Name:            d.Name,
		Description:     d.Description,
		ImageURL:        d.ImageURL,
		Price:           d.Price,
		IsAvailable:     d.IsAvailable,
		Type:            t,
		Entries:         entries,
		HasDiscount:     d.HasDiscount,
		DiscountPercent: d.DiscountPercent,
		CreatedAt:       d.CreatedAt,
		UpdatedAt:       d.UpdatedAt,
	}
}

func shopItemToDTO(m *model.ShopItem) dto.ShopItem {
	itemType := string(m.Type)
	if itemType == "" {
		itemType = string(model.ItemTypeItem)
	}

	entries := make([]dto.KitEntryDTO, len(m.Entries))
	for i, e := range m.Entries {
		entries[i] = dto.KitEntryDTO{
			Name:        e.Name,
			Description: e.Description,
			ImageURL:    e.ImageURL,
			Quantity:    e.Quantity,
		}
	}

	d := dto.ShopItem{
		Code:            m.Code,
		Name:            m.Name,
		Description:     m.Description,
		ImageURL:        m.ImageURL,
		Price:           m.Price,
		IsAvailable:     m.IsAvailable,
		Type:            itemType,
		Entries:         entries,
		HasDiscount:     m.HasDiscount,
		DiscountPercent: m.DiscountPercent,
	}
	if m.Id != "" {
		if oid, err := mongox.ParseObjectID(m.Id); err == nil {
			d.Id = oid
		}
	}
	d.CreatedAt = m.CreatedAt
	d.UpdatedAt = m.UpdatedAt
	return d
}

func purchaseFromDTO(d dto.Purchase) *model.Purchase {
	return &model.Purchase{
		Id:              d.Model.Id.Hex(),
		PlayerID:        d.PlayerID,
		PlayerName:      d.PlayerName,
		ItemID:          d.ItemID,
		ItemName:        d.ItemName,
		PricePaid:       d.PricePaid,
		BasePrice:       d.BasePrice,
		DiscountPercent: d.DiscountPercent,
		Status:          model.PurchaseStatus(d.Status),
		CreatedAt:       d.CreatedAt,
		RefundedAt:      d.RefundedAt,
		IssuedAt:        d.IssuedAt,
		IssuedBy:        d.IssuedBy,
	}
}

// purchaseToDTO builds a BSON-ready Purchase DTO from a domain model, reusing the supplied mongox.Model envelope.
func purchaseToDTO(m mongox.Model, p *model.Purchase) dto.Purchase {
	return dto.Purchase{
		Model:           m,
		PlayerID:        p.PlayerID,
		PlayerName:      p.PlayerName,
		ItemID:          p.ItemID,
		ItemName:        p.ItemName,
		PricePaid:       p.PricePaid,
		BasePrice:       p.BasePrice,
		DiscountPercent: p.DiscountPercent,
		Status:          string(p.Status),
		RefundedAt:      p.RefundedAt,
		IssuedAt:        p.IssuedAt,
		IssuedBy:        p.IssuedBy,
	}
}

func txFromDTO(d dto.Transaction) *model.Transaction {
	return &model.Transaction{
		Id:         d.Id.Hex(),
		PlayerID:   d.PlayerID,
		Amount:     d.Amount,
		Type:       model.TxType(d.Type),
		Reason:     d.Reason,
		PurchaseID: d.PurchaseID,
		CreatedAt:  d.CreatedAt,
	}
}
