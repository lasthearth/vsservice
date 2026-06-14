package model

import "errors"

// ShopItem is a guarded aggregate: it has a New* constructor.
type ShopItem struct {
	Name  string
	Price int64
}

func NewShopItem(name string, price int64) *ShopItem {
	return &ShopItem{Name: name, Price: price} // ok: same package
}

func (s *ShopItem) SetPrice(p int64) error {
	if p < 0 {
		return errors.New("negative")
	}
	s.Price = p // ok: same package (method)
	return nil
}

// Wallet is another guarded aggregate (value-returning constructor).
type Wallet struct {
	Coins int64
}

func NewWallet() Wallet { return Wallet{} } // ok: same package

// KitEntry has NO constructor: literal construction is allowed, but direct
// field mutation outside the package is still forbidden (it is a model struct).
type KitEntry struct {
	Name     string
	Quantity int32
}

// News has neither constructor nor methods, but is still a model struct:
// literal construction is allowed, direct field mutation is not.
type News struct {
	ID    string
	Title string
}
