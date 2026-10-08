package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appearancev1 "github.com/lasthearth/vsservice/gen/appearance/v1"
	"github.com/lasthearth/vsservice/internal/appearance/internal/model"
	"github.com/lasthearth/vsservice/internal/appearance/internal/service"
	"github.com/lasthearth/vsservice/internal/appearance/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/donate/donateuc"
	pkgerr "github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
)

// code is the gRPC code the domain interceptor will answer with.
func code(err error) codes.Code {
	var de *pkgerr.DomainError
	if errors.As(err, &de) {
		return de.Code
	}
	return codes.Unknown
}

type fakeRepo struct {
	saved       map[string]model.Appearance
	bought      []string
	purchaseErr error
	standings   *fakeStandings
}

func (r *fakeRepo) AddPurchase(_ context.Context, _ string, itemID string, _ int64, _ time.Time) error {
	if r.purchaseErr != nil {
		return r.purchaseErr
	}
	r.bought = append(r.bought, itemID)
	// The standing reads purchases from the same store.
	owned := map[string]bool{itemID: true}
	for id := range r.standings.standing.Purchased {
		owned[id] = true
	}
	r.standings.standing = model.Standing{
		Hours:     r.standings.standing.Hours,
		Purchased: owned,
	}
	return nil
}

type fakeWallet struct {
	coins    int64
	debited  int64
	refunded int64
}

func (w *fakeWallet) Debit(_ context.Context, _ string, amount int64, _ string) error {
	if w.coins < amount {
		return donateuc.ErrInsufficientFunds
	}
	w.coins -= amount
	w.debited += amount
	return nil
}

func (w *fakeWallet) Credit(_ context.Context, _, _ string, amount int64, _ string) error {
	w.coins += amount
	w.refunded += amount
	return nil
}

func (r *fakeRepo) List(context.Context) ([]model.Appearance, error) {
	out := make([]model.Appearance, 0, len(r.saved))
	for _, a := range r.saved {
		out = append(out, a)
	}
	return out, nil
}

func (r *fakeRepo) Save(_ context.Context, a *model.Appearance) error {
	r.saved[a.UserId] = *a
	return nil
}

func (r *fakeRepo) Delete(_ context.Context, userID string) error {
	delete(r.saved, userID)
	return nil
}

type fakeStandings struct {
	standing model.Standing
	err      error
}

func (f *fakeStandings) Standing(context.Context, string) (model.Standing, error) {
	return f.standing, f.err
}

func newService(t *testing.T, standing model.Standing) (*service.Service, *fakeRepo, *fakeStandings) {
	svc, repo, standings, _ := newServiceWithWallet(t, standing, 0)
	return svc, repo, standings
}

func newServiceWithWallet(
	t *testing.T,
	standing model.Standing,
	coins int64,
) (*service.Service, *fakeRepo, *fakeStandings, *fakeWallet) {
	t.Helper()
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}
	standings := &fakeStandings{standing: standing}
	repo := &fakeRepo{saved: map[string]model.Appearance{}, standings: standings}
	wallet := &fakeWallet{coins: coins}
	return service.New(service.Opts{
		Logger:    l,
		Repo:      repo,
		Standings: standings,
		Wallet:    wallet,
		Mapper:    &sermapper.MapperImpl{},
	}), repo, standings, wallet
}

func as(uid string) context.Context {
	return interceptor.ContextWithUserID(context.Background(), uid)
}

func request(banner, frame string) *appearancev1.UpdateMyAppearanceRequest {
	return &appearancev1.UpdateMyAppearanceRequest{
		BannerId:     banner,
		BannerEffect: "none",
		FrameId:      frame,
		FrameEffect:  "none",
		TitleKey:     "settler",
	}
}

func TestUpdateSavesForCaller(t *testing.T) {
	svc, repo, _ := newService(t, model.Standing{Hours: 120, Kills: 12})

	got, err := svc.UpdateMyAppearance(as("u1"), request("duel", "bronze"))
	if err != nil {
		t.Fatal(err)
	}
	if got.GetUserId() != "u1" || got.GetBannerId() != "duel" || got.GetFrameId() != "bronze" ||
		got.GetTitleKey() != "settler" || got.GetUpdatedAt() == nil {
		t.Fatalf("unexpected response %+v", got)
	}
	if repo.saved["u1"].BannerId != "duel" {
		t.Fatalf("not saved: %+v", repo.saved)
	}
}

func TestUpdateRejectsLockedItem(t *testing.T) {
	svc, repo, _ := newService(t, model.Standing{Hours: 3})

	_, err := svc.UpdateMyAppearance(as("u1"), request("procession", "gold"))
	if code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %v (%v), want FailedPrecondition", code(err), err)
	}
	if len(repo.saved) != 0 {
		t.Fatalf("locked choice was saved: %+v", repo.saved)
	}
}

func TestUpdateRejectsUnknownItem(t *testing.T) {
	svc, _, _ := newService(t, model.Standing{})

	_, err := svc.UpdateMyAppearance(as("u1"), request("abbey-ruins", "wood"))
	if code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v (%v), want InvalidArgument", code(err), err)
	}
}

func TestUpdateNeedsCaller(t *testing.T) {
	svc, _, _ := newService(t, model.Standing{})

	_, err := svc.UpdateMyAppearance(context.Background(), request("procession", "wood"))
	if code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v (%v), want Unauthenticated", code(err), err)
	}
}

func TestUpdatePassesStandingError(t *testing.T) {
	svc, repo, standings := newService(t, model.Standing{})
	standings.err = errors.New("mongo down")

	if _, err := svc.UpdateMyAppearance(as("u1"), request("procession", "wood")); err == nil {
		t.Fatal("expected an error")
	}
	if len(repo.saved) != 0 {
		t.Fatal("saved despite the standing error")
	}
}

func TestListAndReset(t *testing.T) {
	svc, _, _ := newService(t, model.Standing{})
	for _, uid := range []string{"u1", "u2"} {
		if _, err := svc.UpdateMyAppearance(as(uid), request("procession", "none")); err != nil {
			t.Fatal(err)
		}
	}

	list, err := svc.ListAppearances(context.Background(), &appearancev1.ListAppearancesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.GetAppearances()) != 2 {
		t.Fatalf("listed %d, want 2", len(list.GetAppearances()))
	}

	if _, err := svc.ResetMyAppearance(as("u1"), &appearancev1.ResetMyAppearanceRequest{}); err != nil {
		t.Fatal(err)
	}
	// Resetting twice is fine.
	if _, err := svc.ResetMyAppearance(as("u1"), &appearancev1.ResetMyAppearanceRequest{}); err != nil {
		t.Fatal(err)
	}
	list, _ = svc.ListAppearances(context.Background(), &appearancev1.ListAppearancesRequest{})
	if len(list.GetAppearances()) != 1 || list.GetAppearances()[0].GetUserId() != "u2" {
		t.Fatalf("after reset: %+v", list.GetAppearances())
	}
}

func buy(id string) *appearancev1.BuyBannerRequest {
	return &appearancev1.BuyBannerRequest{BannerId: id}
}

func TestBuyBannerTakesShardsAndUnlocks(t *testing.T) {
	svc, repo, _, wallet := newServiceWithWallet(t, model.Standing{Hours: 10}, 5000)

	got, err := svc.BuyBanner(as("u1"), buy("piper"))
	if err != nil {
		t.Fatal(err)
	}
	if wallet.coins != 2000 || wallet.debited != 3000 {
		t.Fatalf("wallet = %+v", wallet)
	}
	if len(repo.bought) != 1 || repo.bought[0] != "piper" {
		t.Fatalf("bought = %v", repo.bought)
	}
	if ids := got.GetPurchasedBannerIds(); len(ids) != 1 || ids[0] != "piper" {
		t.Fatalf("standing purchases = %v", ids)
	}

	// Now it can be chosen.
	if _, err := svc.UpdateMyAppearance(as("u1"), request("piper", "wood")); err != nil {
		t.Fatalf("bought banner rejected: %v", err)
	}
}

func TestBuyBannerWithoutShards(t *testing.T) {
	svc, repo, _, wallet := newServiceWithWallet(t, model.Standing{}, 2999)

	_, err := svc.BuyBanner(as("u1"), buy("three-dead"))
	if code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %v (%v), want FailedPrecondition", code(err), err)
	}
	if wallet.coins != 2999 || len(repo.bought) != 0 {
		t.Fatalf("something changed: wallet=%+v bought=%v", wallet, repo.bought)
	}
}

func TestBuyBannerTwice(t *testing.T) {
	svc, _, _, wallet := newServiceWithWallet(t, model.Standing{Purchased: map[string]bool{"azure-goat": true}}, 9000)

	_, err := svc.BuyBanner(as("u1"), buy("azure-goat"))
	if code(err) != codes.AlreadyExists {
		t.Fatalf("code = %v (%v), want AlreadyExists", code(err), err)
	}
	if wallet.debited != 0 {
		t.Fatal("charged for an owned banner")
	}
}

func TestBuyBannerThatIsEarned(t *testing.T) {
	svc, _, _, wallet := newServiceWithWallet(t, model.Standing{}, 9000)

	if _, err := svc.BuyBanner(as("u1"), buy("duel")); code(err) != codes.FailedPrecondition {
		t.Fatalf("earned banner: code = %v (%v), want FailedPrecondition", code(err), err)
	}
	if _, err := svc.BuyBanner(as("u1"), buy("nope")); code(err) != codes.InvalidArgument {
		t.Fatalf("unknown banner: code = %v (%v), want InvalidArgument", code(err), err)
	}
	if wallet.debited != 0 {
		t.Fatal("charged for a banner that is not sold")
	}
}

// If the purchase cannot be recorded — a racing second click hit the unique
// index — the shards go back.
func TestBuyBannerRefundsWhenRecordFails(t *testing.T) {
	svc, repo, _, wallet := newServiceWithWallet(t, model.Standing{}, 3000)
	repo.purchaseErr = model.ErrAlreadyOwned

	_, err := svc.BuyBanner(as("u1"), buy("piper"))
	if code(err) != codes.AlreadyExists {
		t.Fatalf("code = %v (%v), want AlreadyExists", code(err), err)
	}
	if wallet.coins != 3000 || wallet.refunded != 3000 {
		t.Fatalf("shards not returned: %+v", wallet)
	}
}

func TestGetMyStanding(t *testing.T) {
	svc, _, _, _ := newServiceWithWallet(t, model.Standing{
		Hours: 12.5, Kills: 3, Deaths: 4, HoursRank: 7, SettlementRole: model.SettlementLeader,
		HungerGamesWins: 2, Referrals: 1, Events: 5, Days: 40,
		Purchased: map[string]bool{"piper": true, "azure-goat": true},
	}, 0)

	got, err := svc.GetMyStanding(as("u1"), &appearancev1.GetMyStandingRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetHours() != 12.5 || got.GetDeaths() != 4 || got.GetSettlementRole() != "leader" ||
		got.GetHungerGamesWins() != 2 || got.GetEvents() != 5 || got.GetDays() != 40 {
		t.Fatalf("unexpected standing %+v", got)
	}
	if ids := got.GetPurchasedBannerIds(); len(ids) != 2 || ids[0] != "azure-goat" || ids[1] != "piper" {
		t.Fatalf("purchases not sorted: %v", ids)
	}

	if _, err := svc.GetMyStanding(context.Background(), &appearancev1.GetMyStandingRequest{}); code(err) != codes.Unauthenticated {
		t.Fatalf("anonymous: code = %v, want Unauthenticated", code(err))
	}
}
