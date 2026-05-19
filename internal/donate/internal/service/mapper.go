package service

import (
	donatev1 "github.com/lasthearth/vsservice/gen/donate/v1"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/goverter"
)

// Mapper converts domain models to proto messages.
type Mapper interface {
	ToShopItemProto(*model.ShopItem) *donatev1.ShopItem
	ToShopItemsProto([]*model.ShopItem) []*donatev1.ShopItem
	ToPurchaseProto(*model.Purchase) *donatev1.Purchase
	ToPurchasesProto([]*model.Purchase) []*donatev1.Purchase
	ToTransactionProto(*model.Transaction) *donatev1.Transaction
	ToTransactionsProto([]*model.Transaction) []*donatev1.Transaction
}

type MapperImpl struct{}

func (m *MapperImpl) ToShopItemProto(s *model.ShopItem) *donatev1.ShopItem {
	if s == nil {
		return nil
	}
	return &donatev1.ShopItem{
		Id:          s.Id,
		Code:        s.Code,
		Name:        s.Name,
		Description: s.Description,
		ImageUrl:    s.ImageURL,
		Price:       s.Price,
		IsAvailable: s.IsAvailable,
		CreatedAt:   goverter.TimeToTimestamp(s.CreatedAt),
		UpdatedAt:   goverter.TimeToTimestamp(s.UpdatedAt),
	}
}

func (m *MapperImpl) ToShopItemsProto(items []*model.ShopItem) []*donatev1.ShopItem {
	result := make([]*donatev1.ShopItem, len(items))
	for i, item := range items {
		result[i] = m.ToShopItemProto(item)
	}
	return result
}

func (m *MapperImpl) ToPurchaseProto(p *model.Purchase) *donatev1.Purchase {
	if p == nil {
		return nil
	}
	return &donatev1.Purchase{
		Id:         p.Id,
		PlayerId:   p.PlayerID,
		PlayerName: p.PlayerName,
		ItemId:     p.ItemID,
		ItemName:   p.ItemName,
		PricePaid:  p.PricePaid,
		Status:     string(p.Status),
		CreatedAt:  goverter.TimeToTimestamp(p.CreatedAt),
		RefundedAt: goverter.TimePtrToTimestamp(p.RefundedAt),
	}
}

func (m *MapperImpl) ToPurchasesProto(purchases []*model.Purchase) []*donatev1.Purchase {
	result := make([]*donatev1.Purchase, len(purchases))
	for i, p := range purchases {
		result[i] = m.ToPurchaseProto(p)
	}
	return result
}

func (m *MapperImpl) ToTransactionProto(t *model.Transaction) *donatev1.Transaction {
	if t == nil {
		return nil
	}
	return &donatev1.Transaction{
		Id:         t.Id,
		PlayerId:   t.PlayerID,
		Amount:     t.Amount,
		Type:       string(t.Type),
		Reason:     t.Reason,
		PurchaseId: t.PurchaseID,
		CreatedAt:  goverter.TimeToTimestamp(t.CreatedAt),
	}
}

func (m *MapperImpl) ToTransactionsProto(txs []*model.Transaction) []*donatev1.Transaction {
	result := make([]*donatev1.Transaction, len(txs))
	for i, tx := range txs {
		result[i] = m.ToTransactionProto(tx)
	}
	return result
}
