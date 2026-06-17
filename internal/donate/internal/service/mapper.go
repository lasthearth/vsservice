//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/donate/internal/service
package service

import (
	donatev1 "github.com/lasthearth/vsservice/gen/donate/v1"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
)

// goverter:converter
// goverter:output:file sermapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTimestamp
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimePtrToTimestamp
// goverter:extend github.com/lasthearth/vsservice/internal/donate/internal/goverter:ItemTypeModelToProto
// goverter:extend github.com/lasthearth/vsservice/internal/donate/internal/goverter:KitEntryModelToProto
// goverter:extend github.com/lasthearth/vsservice/internal/donate/internal/goverter:PrivilegeModelToProto
// goverter:extend github.com/lasthearth/vsservice/internal/donate/internal/goverter:PtrStringToString
// goverter:extend github.com/lasthearth/vsservice/internal/donate/internal/goverter:PurchaseStatusToString
// goverter:extend github.com/lasthearth/vsservice/internal/donate/internal/goverter:TxTypeToString
// goverter:extend github.com/lasthearth/vsservice/internal/donate/internal/goverter:ShopItemEffectivePrice
type Mapper interface {
	// goverter:ignore state sizeCache unknownFields DiscountActive
	// goverter:map ImageURL ImageUrl
	// goverter:map Type ItemType | github.com/lasthearth/vsservice/internal/donate/internal/goverter:ItemTypeModelToProto
	// goverter:map . EffectivePrice | github.com/lasthearth/vsservice/internal/donate/internal/goverter:ShopItemEffectivePrice
	ToShopItemProto(*model.ShopItem) *donatev1.ShopItem
	ToShopItemsProto([]*model.ShopItem) []*donatev1.ShopItem

	// goverter:ignore state sizeCache unknownFields
	// goverter:map PlayerID PlayerId
	// goverter:map ItemID ItemId
	// goverter:map Status Status | github.com/lasthearth/vsservice/internal/donate/internal/goverter:PurchaseStatusToString
	// goverter:map IssuedBy IssuedBy | github.com/lasthearth/vsservice/internal/donate/internal/goverter:PtrStringToString
	ToPurchaseProto(*model.Purchase) *donatev1.Purchase
	ToPurchasesProto([]*model.Purchase) []*donatev1.Purchase

	// goverter:ignore state sizeCache unknownFields
	// goverter:map PlayerID PlayerId
	// goverter:map PurchaseID PurchaseId
	// goverter:map Type Type | github.com/lasthearth/vsservice/internal/donate/internal/goverter:TxTypeToString
	ToTransactionProto(*model.Transaction) *donatev1.Transaction
	ToTransactionsProto([]*model.Transaction) []*donatev1.Transaction

	// goverter:ignore state sizeCache unknownFields
	// goverter:map PlayerID PlayerId
	ToWalletBalanceProto(*model.Wallet) *donatev1.WalletBalance
	ToWalletBalancesProto([]*model.Wallet) []*donatev1.WalletBalance
}
