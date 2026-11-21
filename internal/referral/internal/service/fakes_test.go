package service

import (
	"context"
	"sort"
	"testing"
	"time"

	referralv1 "github.com/lasthearth/vsservice/gen/referral/v1"
	"github.com/lasthearth/vsservice/internal/donate/donateuc"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/referral/internal/ierror"
	"github.com/lasthearth/vsservice/internal/referral/internal/model"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
)

const (
	testReferrerID = "referrer1"
	testRefereeID  = "referee1"
	testOtherID    = "referee2"
	testCode       = "ABCD1234"

	// 200 splits into 50 on verification and 150 on the join, which keeps the
	// two shares apart from the referee's own reward below.
	testReferrerReward = int64(200)
	testRefereeReward  = int64(100)
)

// claimedReward is one claim the worker made on the repository.
type claimedReward struct {
	refereePlayerID string
	stage           model.RewardStage
	coins           int64
}

// fakeRepo is a hand-written stand-in for Repository. It reproduces the three
// contracts the service relies on: referee_player_id is unique, a stage can be
// claimed once and only by one caller, and a claim that is released can be
// claimed again.
type fakeRepo struct {
	codes  map[string]*model.ReferralCode
	events map[string]*model.ReferralEvent // keyed by referee id

	verified map[string]bool
	joined   map[string]bool

	createErr   error
	listErr     error
	progressErr error
	claimErr    error
	releaseErr  error
	deferErr    error

	claimed  []claimedReward
	released []claimedReward
	deferred []string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		codes:    map[string]*model.ReferralCode{},
		events:   map[string]*model.ReferralEvent{},
		verified: map[string]bool{},
		joined:   map[string]bool{},
	}
}

func (f *fakeRepo) withCode(playerID, code string) *fakeRepo {
	f.codes[code] = model.ReconstituteReferralCode("c-"+playerID, playerID, "", code, time.Now())
	return f
}

func (f *fakeRepo) withEvent(referrerPlayerID, refereePlayerID string, referrerReward, refereeReward int64) *fakeRepo {
	f.events[refereePlayerID] = model.ReconstituteReferralEvent(
		"e-"+refereePlayerID,
		referrerPlayerID,
		refereePlayerID,
		referrerReward,
		refereeReward,
		0,
		false,
		false,
		false,
		0,
		time.Now(),
		time.Now(),
	)
	return f
}

func (f *fakeRepo) GetCodeByPlayerID(_ context.Context, playerID string) (*model.ReferralCode, error) {
	for _, c := range f.codes {
		if c.PlayerID == playerID {
			return c, nil
		}
	}
	return nil, ierror.ErrNotFound
}

func (f *fakeRepo) GetCodeByCode(_ context.Context, code string) (*model.ReferralCode, error) {
	c, ok := f.codes[code]
	if !ok {
		return nil, ierror.ErrNotFound
	}
	return c, nil
}

func (f *fakeRepo) UpsertCode(_ context.Context, code *model.ReferralCode) (*model.ReferralCode, error) {
	if existing, ok := f.codes[code.Code]; ok {
		return existing, nil
	}
	f.codes[code.Code] = code
	return code, nil
}

func (f *fakeRepo) CreateEvent(_ context.Context, event *model.ReferralEvent) error {
	if f.createErr != nil {
		return f.createErr
	}
	if _, ok := f.events[event.RefereePlayerID]; ok {
		return ierror.ErrAlreadyReferred
	}
	f.events[event.RefereePlayerID] = event
	return nil
}

func (f *fakeRepo) HasReferee(_ context.Context, refereePlayerID string) (bool, error) {
	_, ok := f.events[refereePlayerID]
	return ok, nil
}

func (f *fakeRepo) GetStatsByPlayerID(_ context.Context, playerID string) (int64, int64, error) {
	var total, coins int64
	for _, e := range f.events {
		if e.ReferrerPlayerID == playerID {
			total++
			coins += e.CoinsAwarded
		}
	}
	return total, coins, nil
}

// ListUnpaidEvents mirrors the Mongo filter: an event that owes nothing, and an
// event written before the two-stage payout, do not reach the worker.
func (f *fakeRepo) ListUnpaidEvents(
	_ context.Context,
	now time.Time,
	limit int64,
) ([]*model.ReferralEvent, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}

	events := make([]*model.ReferralEvent, 0, len(f.events))
	for _, e := range f.events {
		if e.ReferrerReward <= 0 {
			continue
		}
		if e.VerificationPaid && e.JoinPaid && e.RefereePaid {
			continue
		}
		if e.NextCheckAt.After(now) {
			continue
		}
		events = append(events, e)
	}

	sort.Slice(events, func(i, j int) bool {
		if !events[i].NextCheckAt.Equal(events[j].NextCheckAt) {
			return events[i].NextCheckAt.Before(events[j].NextCheckAt)
		}
		return events[i].RefereePlayerID < events[j].RefereePlayerID
	})

	if limit > 0 && int64(len(events)) > limit {
		events = events[:limit]
	}
	return events, nil
}

func (f *fakeRepo) ClaimReward(
	_ context.Context,
	refereePlayerID string,
	stage model.RewardStage,
	coins int64,
) (bool, error) {
	if f.claimErr != nil {
		return false, f.claimErr
	}

	event, ok := f.events[refereePlayerID]
	if !ok {
		return false, nil
	}
	if event.Paid(stage) {
		return false, nil
	}

	// The stage is unpaid, so MarkPaid cannot refuse it: a failure here would be
	// a bug in the test, not in the code under test.
	_ = event.MarkPaid(stage)

	f.claimed = append(f.claimed, claimedReward{refereePlayerID: refereePlayerID, stage: stage, coins: coins})
	return true, nil
}

func (f *fakeRepo) ReleaseReward(
	_ context.Context,
	refereePlayerID string,
	stage model.RewardStage,
	coins int64,
) error {
	if f.releaseErr != nil {
		return f.releaseErr
	}

	event, ok := f.events[refereePlayerID]
	if !ok {
		return nil
	}
	if err := event.UnmarkPaid(stage); err != nil {
		return err
	}

	f.released = append(f.released, claimedReward{refereePlayerID: refereePlayerID, stage: stage, coins: coins})
	return nil
}

// DeferEvent only records the call: the fake hands out its stored pointers, so the
// schedule the worker put on the model is already what the next pass reads, the
// same way ListUnpaidEvents and ClaimReward share state here.
func (f *fakeRepo) DeferEvent(_ context.Context, event *model.ReferralEvent) error {
	if f.deferErr != nil {
		return f.deferErr
	}

	f.deferred = append(f.deferred, event.RefereePlayerID)

	return nil
}

func (f *fakeRepo) GetRefereeProgress(_ context.Context, refereePlayerID string) (bool, bool, error) {
	if f.progressErr != nil {
		return false, false, f.progressErr
	}
	return f.verified[refereePlayerID], f.joined[refereePlayerID], nil
}

// credit records one donate ledger row the way CreateCreditTransaction does.
type credit struct {
	playerID string
	amount   int64
	reason   string
}

// fakeWallet is a hand-written stand-in for donateuc.WalletRepo. addCoinsErr
// fails the wallet increment for the named players, which is how a test drives
// Credit down its "nothing moved" path.
type fakeWallet struct {
	balances    map[string]int64
	credits     []credit
	addCoinsErr map[string]error
}

func newFakeWallet() *fakeWallet {
	return &fakeWallet{
		balances:    map[string]int64{},
		addCoinsErr: map[string]error{},
	}
}

func (f *fakeWallet) AddCoinsToWallet(_ context.Context, playerID, _ string, amount int64) (int64, error) {
	if err := f.addCoinsErr[playerID]; err != nil {
		return 0, err
	}
	f.balances[playerID] += amount
	return f.balances[playerID], nil
}

func (f *fakeWallet) CreateCreditTransaction(_ context.Context, playerID string, amount int64, reason string) error {
	f.credits = append(f.credits, credit{playerID: playerID, amount: amount, reason: reason})
	return nil
}

func (f *fakeWallet) WithdrawCoins(_ context.Context, _ string, _ int64) error { return nil }

func (f *fakeWallet) CreateDebitTransaction(_ context.Context, _ string, _ int64, _ string) error {
	return nil
}

func testLogger(t *testing.T) logger.Logger {
	t.Helper()

	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func newService(t *testing.T, repo Repository) *Service {
	t.Helper()

	return New(Opts{
		Cfg:    config.Config{ReferralCoinsReward: testReferrerReward, ReferralRefereeCoinsReward: testRefereeReward},
		DbRepo: repo,
	})
}

func newWorker(t *testing.T, repo Repository, wallet donateuc.WalletRepo) *PayoutWorker {
	t.Helper()

	return NewPayoutWorker(PayoutOpts{
		Log:      testLogger(t),
		Cfg:      config.Config{ReferralPayoutInterval: time.Minute},
		DbRepo:   repo,
		DonateUC: donateuc.NewAddCoinsUseCase(donateuc.Opts{Repo: wallet}),
	})
}

func refereeCtx() context.Context {
	return withUserID(context.Background(), testRefereeID)
}

// withUserID puts playerID in the request context, the way the JWT interceptor
// does before a handler runs.
func withUserID(ctx context.Context, playerID string) context.Context {
	return interceptor.ContextWithUserID(ctx, playerID)
}

func useCode(code string) *referralv1.UseReferralCodeRequest {
	return &referralv1.UseReferralCodeRequest{Code: code}
}
