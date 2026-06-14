package service

import "sample/internal/shop/internal/model"

func build() {
	// Struct literals of a constructor-guarded aggregate -> must use New*.
	_ = &model.ShopItem{Name: "x"} // want `construct ShopItem via its NewShopItem constructor, not a struct literal`
	_ = model.ShopItem{}           // want `construct ShopItem via its NewShopItem constructor, not a struct literal`
	_ = model.Wallet{Coins: 1}     // want `construct Wallet via its NewWallet constructor, not a struct literal`

	// Constructor + method path is fine.
	it := model.NewShopItem("a", 1)
	_ = it.SetPrice(5) // ok: method

	// Direct field writes on a model struct -> must use a method.
	it.Price = 9 // want `mutate ShopItem via its methods, not direct field assignment`
	it.Price++   // want `mutate ShopItem via its methods, not direct field assignment`

	w := model.NewWallet()
	w.Coins += 5 // want `mutate Wallet via its methods, not direct field assignment`

	// Value object without a constructor: literal build is allowed...
	e := model.KitEntry{Name: "ok"}
	e.Quantity = 3 // want `mutate KitEntry via its methods, not direct field assignment`
	_ = e

	// Model with neither constructor nor methods: literal allowed, write not.
	n := model.News{Title: "t"}
	n.Title = "u" // want `mutate News via its methods, not direct field assignment`
	_ = n
}
