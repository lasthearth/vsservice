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
	mapper     Mapper
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
	Mapper   Mapper
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
		mapper:     opts.Mapper,
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
	})
	createIndex(txColl, mgo.IndexModel{
		Keys: bson.D{{Key: "player_id", Value: 1}},
	})
}

func walletFromDTO(d dto.Wallet) *model.Wallet {
	return model.ReconstituteWallet(d.Model.Id.Hex(), d.PlayerID, d.PlayerName, d.Coins, d.CreatedAt, d.UpdatedAt)
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

	privileges := make([]model.Privilege, len(d.Privileges))
	for i, p := range d.Privileges {
		privileges[i] = model.Privilege{
			Text: p.Text,
			Icon: p.Icon,
		}
	}

	return model.ReconstituteShopItem(
		d.Id.Hex(), d.Code, d.Name, d.Description, d.ImageURL,
		d.Price, d.IsAvailable, t, entries,
		d.HasDiscount, d.DiscountPercent, privileges,
		d.DiscountStartsAt, d.DiscountEndsAt, d.CreatedAt, d.UpdatedAt,
	)
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

	privileges := make([]dto.PrivilegeDTO, len(m.Privileges))
	for i, p := range m.Privileges {
		privileges[i] = dto.PrivilegeDTO{
			Text: p.Text,
			Icon: p.Icon,
		}
	}

	d := dto.ShopItem{
		Code:             m.Code,
		Name:             m.Name,
		Description:      m.Description,
		ImageURL:         m.ImageURL,
		Price:            m.Price,
		IsAvailable:      m.IsAvailable,
		Type:             itemType,
		Entries:          entries,
		HasDiscount:      m.HasDiscount,
		DiscountPercent:  m.DiscountPercent,
		Privileges:       privileges,
		DiscountStartsAt: m.DiscountStartsAt,
		DiscountEndsAt:   m.DiscountEndsAt,
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
	return model.ReconstitutePurchase(
		d.Model.Id.Hex(), d.PlayerID, d.PlayerName, d.ItemID, d.ItemName,
		d.PricePaid, d.BasePrice, d.DiscountPercent,
		model.PurchaseStatus(d.Status), d.CreatedAt, d.RefundedAt, d.IssuedAt, d.IssuedBy,
	)
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
	return model.ReconstituteTransaction(d.Id.Hex(), d.PlayerID, d.Amount, model.TxType(d.Type), d.Reason, d.PurchaseID, d.CreatedAt)
}
