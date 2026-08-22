package service

import (
	"context"
	"strings"
	"testing"
	"time"

	hgv1 "github.com/lasthearth/vsservice/gen/hungergames/v1"
	"github.com/lasthearth/vsservice/internal/donate/donateuc"
	"github.com/lasthearth/vsservice/internal/hungergames/internal/ierror"
	"github.com/lasthearth/vsservice/internal/hungergames/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// resetRepo records the order of the calls ResetSeason makes, so the test can
// assert that the season is claimed before any coins move. The embedded nil
// interface makes an unexpected call panic and name itself.
type resetRepo struct {
	Repository

	active   *model.Season
	stats    []*model.PlayerStats
	closeErr error
	// calls is shared with creditRecorder so the ordering of the season claim
	// relative to the wallet credits is observable.
	calls *[]string
}

func (r *resetRepo) record(name string) { *r.calls = append(*r.calls, name) }

func (r *resetRepo) GetActiveSeason(context.Context) (*model.Season, error) {
	r.record("GetActiveSeason")
	if r.active == nil {
		return nil, ierror.ErrNoActiveSeason
	}
	return r.active, nil
}

func (r *resetRepo) ListAllPlayerStatsByELO(context.Context) ([]*model.PlayerStats, error) {
	r.record("ListAllPlayerStatsByELO")
	return r.stats, nil
}

func (r *resetRepo) CloseSeason(context.Context, string) error {
	r.record("CloseSeason")
	return r.closeErr
}

func (r *resetRepo) CreateSeasonResults(context.Context, []*model.SeasonResult) error {
	r.record("CreateSeasonResults")
	return nil
}

func (r *resetRepo) DeleteAllPlayerStats(context.Context) error {
	r.record("DeleteAllPlayerStats")
	return nil
}

// creditRecorder captures wallet credits.
type creditRecorder struct {
	credited []string
	calls    *[]string
}

func (c *creditRecorder) AddCoinsToWallet(_ context.Context, playerID, _ string, _ int64) (int64, error) {
	c.credited = append(c.credited, playerID)
	*c.calls = append(*c.calls, "Credit:"+playerID)
	return 0, nil
}

func (c *creditRecorder) CreateCreditTransaction(context.Context, string, int64, string) error {
	return nil
}

func newResetService(t *testing.T, repo Repository, wallet donateuc.WalletRepo) *Service {
	t.Helper()
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}
	return New(Opts{
		Repo:     repo,
		DonateUC: donateuc.NewAddCoinsUseCase(donateuc.Opts{Repo: wallet}),
		Logger:   l,
	})
}

func resetRequest() *hgv1.ResetSeasonRequest {
	return &hgv1.ResetSeasonRequest{
		Rewards: []*hgv1.SeasonReward{{Rank: 1, Coins: 10000}},
	}
}

// The season must be closed BEFORE any reward is credited. Credit is an atomic
// $inc and cannot be reversed, so paying first let two concurrent resets pay
// every reward twice.
func TestResetSeasonClaimsTheSeasonBeforePayingRewards(t *testing.T) {
	var calls []string
	repo := &resetRepo{
		calls:  &calls,
		active: model.ReconstituteSeason("s1", 3, time.Now(), nil),
		stats:  []*model.PlayerStats{model.ReconstitutePlayerStats("ps1", "p1", "Player1", 1500, 5, 20, "s1", time.Now(), time.Now())},
	}
	wallet := &creditRecorder{calls: &calls}
	svc := newResetService(t, repo, wallet)

	if _, err := svc.ResetSeason(context.Background(), resetRequest()); err != nil {
		t.Fatalf("ResetSeason: %v", err)
	}

	closeIdx, firstCreditIdx := -1, -1
	for i, c := range calls {
		if c == "CloseSeason" && closeIdx == -1 {
			closeIdx = i
		}
		if strings.HasPrefix(c, "Credit:") && firstCreditIdx == -1 {
			firstCreditIdx = i
		}
	}
	if closeIdx == -1 {
		t.Fatalf("CloseSeason was never called: %v", calls)
	}
	if firstCreditIdx == -1 {
		t.Fatalf("no reward was credited: %v", calls)
	}
	if closeIdx > firstCreditIdx {
		t.Errorf("want CloseSeason before the first credit, got %v", calls)
	}
	if len(wallet.credited) != 1 {
		t.Fatalf("want 1 credit, got %d", len(wallet.credited))
	}
}

// The loser of a concurrent reset must pay nothing.
func TestResetSeasonPaysNothingWhenTheClaimIsLost(t *testing.T) {
	var calls []string
	repo := &resetRepo{
		calls:    &calls,
		active:   model.ReconstituteSeason("s1", 3, time.Now(), nil),
		stats:    []*model.PlayerStats{model.ReconstitutePlayerStats("ps1", "p1", "Player1", 1500, 5, 20, "s1", time.Now(), time.Now())},
		closeErr: ierror.ErrSeasonAlreadyClosed,
	}
	wallet := &creditRecorder{calls: &calls}
	svc := newResetService(t, repo, wallet)

	_, err := svc.ResetSeason(context.Background(), resetRequest())
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("want AlreadyExists, got %v (err=%v)", status.Code(err), err)
	}

	if len(wallet.credited) != 0 {
		t.Errorf("want no credits after a lost claim, got %v", wallet.credited)
	}
	for _, c := range calls {
		if c == "CreateSeasonResults" || c == "DeleteAllPlayerStats" {
			t.Errorf("want no writes after a lost claim, got %v", calls)
		}
	}
}
