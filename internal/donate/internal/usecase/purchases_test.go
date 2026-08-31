package usecase_test

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/donate/internal/ierror"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
	"github.com/lasthearth/vsservice/internal/donate/internal/usecase"
)

// fakeRepo is a hand-written stand-in for usecase.PurchaseRepo. It reproduces
// the contracts the Mongo implementation offers: UpdateWallet loads, applies and
// stores; AddCoinsToWallet is an upsert that never blanks a stored name;
// CreatePurchase assigns an id.
type fakeRepo struct {
	items     map[string]*model.ShopItem
	wallets   map[string]*model.Wallet
	purchases map[string]*model.Purchase
	txs       []*model.Transaction

	nextID int

	// Failure injection, per method.
	getItemErr    error
	walletErr     error
	addCoinsErr   error
	createPurErr  error
	updatePurErr  error
	createTxErr   error
	walletCalls   int
	addCoinsCalls int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		items:     map[string]*model.ShopItem{},
		wallets:   map[string]*model.Wallet{},
		purchases: map[string]*model.Purchase{},
	}
}

func (f *fakeRepo) id() string {
	f.nextID++
	return string(rune('a' + f.nextID - 1))
}

func (f *fakeRepo) GetShopItem(_ context.Context, id string) (*model.ShopItem, error) {
	if f.getItemErr != nil {
		return nil, f.getItemErr
	}
	item, ok := f.items[id]
	if !ok {
		return nil, ierror.ErrNotFound
	}
	return item, nil
}

func (f *fakeRepo) GetWalletByPlayerID(_ context.Context, playerID string) (*model.Wallet, error) {
	w, ok := f.wallets[playerID]
	if !ok {
		return nil, ierror.ErrNotFound
	}
	return w, nil
}

func (f *fakeRepo) UpdateWallet(
	ctx context.Context,
	playerID string,
	updateFn func(context.Context, *model.Wallet) (*model.Wallet, error),
) error {
	f.walletCalls++
	if f.walletErr != nil {
		return f.walletErr
	}
	w, ok := f.wallets[playerID]
	if !ok {
		return ierror.ErrNotFound
	}
	updated, err := updateFn(ctx, w)
	if err != nil {
		return err
	}
	f.wallets[playerID] = updated
	return nil
}

func (f *fakeRepo) AddCoinsToWallet(_ context.Context, playerID, playerName string, amount int64) (int64, error) {
	f.addCoinsCalls++
	if f.addCoinsErr != nil {
		return 0, f.addCoinsErr
	}
	w, ok := f.wallets[playerID]
	if !ok {
		w = model.NewWallet(playerID, playerName)
		f.wallets[playerID] = w
	}
	if err := w.Deposit(amount); err != nil {
		return 0, err
	}
	return w.Coins, nil
}

func (f *fakeRepo) CreatePurchase(_ context.Context, p *model.Purchase) (*model.Purchase, error) {
	if f.createPurErr != nil {
		return nil, f.createPurErr
	}
	p.MarkCreated(f.id(), time.Now())
	f.purchases[p.Id] = p
	return p, nil
}

func (f *fakeRepo) UpdatePurchase(
	ctx context.Context,
	id string,
	updateFn func(context.Context, *model.Purchase) (*model.Purchase, error),
) (*model.Purchase, error) {
	if f.updatePurErr != nil {
		return nil, f.updatePurErr
	}
	p, ok := f.purchases[id]
	if !ok {
		return nil, ierror.ErrNotFound
	}
	updated, err := updateFn(ctx, p)
	if err != nil {
		return nil, err
	}
	f.purchases[id] = updated
	return updated, nil
}

func (f *fakeRepo) CreateTransaction(_ context.Context, tx *model.Transaction) (*model.Transaction, error) {
	if f.createTxErr != nil {
		return nil, f.createTxErr
	}
	tx.MarkCreated(f.id(), time.Now())
	f.txs = append(f.txs, tx)
	return tx, nil
}

// withWallet seeds a wallet holding coins.
func (f *fakeRepo) withWallet(playerID, playerName string, coins int64) *fakeRepo {
	f.wallets[playerID] = model.ReconstituteWallet("w-"+playerID, playerID, playerName, coins, time.Time{}, time.Time{})
	return f
}

// withItem seeds an available item at the given price.
func (f *fakeRepo) withItem(id, name string, price int64) *model.ShopItem {
	item := model.NewShopItem("code-"+id, name, "", "", price)
	item.MarkCreated(id, time.Now())
	f.items[id] = item
	return item
}

// withKitItem seeds an available kit-type item at the given price. Code doubles
// as the kit code the mail composer expands.
func (f *fakeRepo) withKitItem(id, name, code string, price int64) *model.ShopItem {
	item := model.NewKitShopItem(code, name, "", "", price, []model.KitEntry{{Name: "x", Quantity: 1}})
	item.MarkCreated(id, time.Now())
	f.items[id] = item
	return item
}

// withPurchase seeds a stored purchase and returns it.
func (f *fakeRepo) withPurchase(id, playerID, playerName string, pricePaid int64) *model.Purchase {
	p := model.NewPurchase(playerID, playerName, "item-1", "Sword", pricePaid, pricePaid, 0)
	p.MarkCreated(id, time.Now())
	f.purchases[id] = p
	return p
}

func newPurchases(repo *fakeRepo) *usecase.Purchases {
	return usecase.NewPurchases(usecase.Opts{Repo: repo, Seq: usecase.Inline{}, Mail: &fakeMail{}})
}

// fakeMail is a hand-written stand-in for usecase.MailComposer. It records the
// composed mails so tests can assert which path (item vs kit) was taken.
type fakeMail struct {
	itemCalls []itemMailCall
	kitCalls  []kitMailCall
	err       error
}

type itemMailCall struct {
	recipient, title, body, purchaseID string
	items                              []usecase.ItemSpec
}

type kitMailCall struct {
	recipient, kitID, title, body, purchaseID string
}

func (f *fakeMail) ComposeItemMail(_ context.Context, recipient, title, body, purchaseID string, items []usecase.ItemSpec) error {
	if f.err != nil {
		return f.err
	}
	f.itemCalls = append(f.itemCalls, itemMailCall{recipient, title, body, purchaseID, items})
	return nil
}

func (f *fakeMail) ComposeKitMail(_ context.Context, recipient, kitID, title, body, purchaseID string) error {
	if f.err != nil {
		return f.err
	}
	f.kitCalls = append(f.kitCalls, kitMailCall{recipient, kitID, title, body, purchaseID})
	return nil
}

func newPurchasesWithMail(repo *fakeRepo, mail *fakeMail) *usecase.Purchases {
	return usecase.NewPurchases(usecase.Opts{Repo: repo, Seq: usecase.Inline{}, Mail: mail})
}
