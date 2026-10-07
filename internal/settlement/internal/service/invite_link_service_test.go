package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/settlement/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement/internal/service"
	"github.com/lasthearth/vsservice/internal/settlement/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.uber.org/zap"
)

// linkNow is real time: the handlers read the clock themselves.
var linkNow = time.Now()

// inviteRepo fakes the slice of the repository the invite-link handlers use.
// Embedding the interface makes any other call panic, which pins the handlers
// to that slice.
type inviteRepo struct {
	service.SettlementRepository
	set       *model.Settlement
	links     map[string]*model.InviteLink
	active    int64
	created   *model.InviteLink
	joined    *model.InviteLink
	getByCode *model.InviteLink
}

func (r *inviteRepo) GetInviteLinkByCode(_ context.Context, code string) (*model.InviteLink, error) {
	if r.getByCode == nil || r.getByCode.Code != code {
		return nil, ierror.ErrInviteLinkNotFound
	}
	return r.getByCode, nil
}

func (r *inviteRepo) GetSettlement(_ context.Context, id string) (*model.Settlement, error) {
	if r.set == nil || r.set.Id != id {
		return nil, ierror.ErrNotFound
	}
	return r.set, nil
}

func (r *inviteRepo) CountActiveInviteLinks(context.Context, string, time.Time) (int64, error) {
	return r.active, nil
}

func (r *inviteRepo) CreateInviteLink(_ context.Context, link *model.InviteLink) (*model.InviteLink, error) {
	r.created = link
	return link, nil
}

// UpdateInviteLink mirrors production's {_id, settlement_id} filter: a link that
// belongs to another settlement must not be reachable through this one.
func (r *inviteRepo) UpdateInviteLink(
	ctx context.Context,
	settlementID, linkID string,
	fn func(context.Context, *model.InviteLink) (*model.InviteLink, error),
) (*model.InviteLink, error) {
	link, ok := r.links[linkID]
	if !ok || link.SettlementId != settlementID {
		return nil, ierror.ErrInviteLinkNotFound
	}
	working := *link
	return fn(ctx, &working)
}

func (r *inviteRepo) JoinByInviteLink(
	ctx context.Context,
	code, _ string,
	fn func(context.Context, *model.InviteLink) (*model.InviteLink, error),
) (*model.InviteLink, error) {
	for _, link := range r.links {
		if link.Code == code {
			working := *link
			updated, err := fn(ctx, &working)
			r.joined = updated
			return updated, err
		}
	}
	return nil, ierror.ErrInviteLinkNotFound
}

func newInviteService(t *testing.T) (*service.Service, *inviteRepo) {
	t.Helper()
	repo := &inviteRepo{
		set: &model.Settlement{
			Id:   "s1",
			Name: "Северный Оплот",
			Members: []model.Member{
				{UserId: "owner", RoleIds: []string{model.OwnerRoleId}},
				{UserId: "resident", RoleIds: []string{}},
			},
		},
		links: map[string]*model.InviteLink{},
	}
	zc := zap.NewProductionConfig()
	log, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}
	return service.New(service.Opts{DbRepo: repo, Mapper: &sermapper.MapperImpl{}, Log: log}), repo
}

func asUser(uid string) context.Context {
	return interceptor.ContextWithUserID(context.Background(), uid)
}

func TestCreateInviteLinkNeedsInvitePermission(t *testing.T) {
	svc, _ := newInviteService(t)

	_, err := svc.CreateInviteLink(asUser("resident"), &settlementv1.CreateInviteLinkRequest{SettlementId: "s1", TtlHours: 24})
	if !errors.Is(err, ierror.ErrNotLeader) {
		t.Fatalf("plain resident: want ErrNotLeader, got %v", err)
	}
}

func TestCreateInviteLinkLimit(t *testing.T) {
	svc, repo := newInviteService(t)
	repo.active = model.MaxActiveInviteLinks

	_, err := svc.CreateInviteLink(asUser("owner"), &settlementv1.CreateInviteLinkRequest{SettlementId: "s1", TtlHours: 24})
	if !errors.Is(err, ierror.ErrInviteLinkLimit) {
		t.Fatalf("want ErrInviteLinkLimit, got %v", err)
	}
}

func TestCreateInviteLink(t *testing.T) {
	svc, repo := newInviteService(t)

	got, err := svc.CreateInviteLink(asUser("owner"), &settlementv1.CreateInviteLinkRequest{
		SettlementId: "s1",
		TtlHours:     48,
		MaxUses:      5,
	})
	if err != nil {
		t.Fatal(err)
	}

	if got.GetStatus() != settlementv1.InviteLinkStatus_INVITE_LINK_STATUS_ACTIVE {
		t.Fatalf("want active, got %v", got.GetStatus())
	}
	if got.GetCode() == "" || got.GetCode() != repo.created.Code {
		t.Fatalf("code not returned: %q", got.GetCode())
	}
	if got.GetCreatedBy() != "owner" || got.GetMaxUses() != 5 {
		t.Fatalf("unexpected link %+v", got)
	}
	if d := got.GetExpiresAt().AsTime().Sub(linkNow); d < 48*time.Hour || d > 48*time.Hour+time.Minute {
		t.Fatalf("expires_at %v", got.GetExpiresAt().AsTime())
	}
}

func TestRevokeInviteLink(t *testing.T) {
	svc, repo := newInviteService(t)
	link, _ := model.NewInviteLink("s1", "owner", time.Hour, 0, linkNow)
	repo.links["l1"] = link

	got, err := svc.RevokeInviteLink(asUser("owner"), &settlementv1.RevokeInviteLinkRequest{SettlementId: "s1", LinkId: "l1"})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetStatus() != settlementv1.InviteLinkStatus_INVITE_LINK_STATUS_REVOKED || got.GetRevokedAt() == nil {
		t.Fatalf("link not revoked: %+v", got)
	}
}

func TestJoinByInviteLinkRefusesUnusableLinks(t *testing.T) {
	svc, repo := newInviteService(t)

	expired, _ := model.NewInviteLink("s1", "owner", time.Hour, 0, linkNow.Add(-2*time.Hour))
	used, _ := model.NewInviteLink("s1", "owner", 0, 1, linkNow)
	_ = used.Use(linkNow)
	revoked, _ := model.NewInviteLink("s1", "owner", 0, 0, linkNow)
	revoked.Revoke(linkNow)
	repo.links = map[string]*model.InviteLink{"a": expired, "b": used, "c": revoked}

	cases := map[string]error{
		expired.Code: ierror.ErrInviteLinkExpired,
		used.Code:    ierror.ErrInviteLinkExhausted,
		revoked.Code: ierror.ErrInviteLinkRevoked,
		"NOSUCHCODE": ierror.ErrInviteLinkNotFound,
	}
	for code, want := range cases {
		_, err := svc.JoinByInviteLink(asUser("newcomer"), &settlementv1.JoinByInviteLinkRequest{Code: code})
		if !errors.Is(err, want) {
			t.Errorf("code %s: want %v, got %v", code, want, err)
		}
	}
}

func TestGetInviteLinkPreview(t *testing.T) {
	svc, repo := newInviteService(t)
	link, _ := model.NewInviteLink("s1", "owner", 0, 10, linkNow)
	repo.links["l1"] = link
	repo.getByCode = link

	got, err := svc.GetInviteLink(context.Background(), &settlementv1.GetInviteLinkRequest{Code: link.Code})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetSettlement().GetName() != "Северный Оплот" || got.GetMaxUses() != 10 || got.GetExpiresAt() != nil {
		t.Fatalf("unexpected preview %+v", got)
	}
	if got.GetStatus() != settlementv1.InviteLinkStatus_INVITE_LINK_STATUS_ACTIVE {
		t.Fatalf("want active, got %v", got.GetStatus())
	}
}

// TestJoinByInviteLinkSucceeds covers the success path: the refusal cases were
// the only ones tested, so a handler that returned an empty settlement id or
// forgot to spend the use still passed.
func TestJoinByInviteLinkSucceeds(t *testing.T) {
	svc, repo := newInviteService(t)
	link, _ := model.NewInviteLink("s1", "owner", time.Hour, 5, linkNow)
	repo.links["l1"] = link

	got, err := svc.JoinByInviteLink(asUser("newcomer"), &settlementv1.JoinByInviteLinkRequest{Code: link.Code})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetSettlementId() != "s1" {
		t.Fatalf("want settlement s1 in the response, got %q", got.GetSettlementId())
	}
	if repo.joined == nil || repo.joined.Uses != 1 {
		t.Fatalf("the join must spend exactly one use, got %+v", repo.joined)
	}
}

// TestRevokeInviteLinkIsScopedToSettlement pins the cross-settlement isolation
// the real filter {_id, settlement_id} provides: holding invite_member in one
// settlement must not let a caller revoke another settlement's link by id.
func TestRevokeInviteLinkIsScopedToSettlement(t *testing.T) {
	svc, repo := newInviteService(t)
	foreign, _ := model.NewInviteLink("s2", "other-owner", time.Hour, 0, linkNow)
	repo.links["foreign"] = foreign

	_, err := svc.RevokeInviteLink(asUser("owner"), &settlementv1.RevokeInviteLinkRequest{
		SettlementId: "s1",
		LinkId:       "foreign",
	})
	if !errors.Is(err, ierror.ErrInviteLinkNotFound) {
		t.Fatalf("want ErrInviteLinkNotFound for another settlement's link, got %v", err)
	}
	if foreign.RevokedAt != nil {
		t.Fatal("the other settlement's link must not be revoked")
	}
}

// TestGetInviteLinkPreviewReportsWhyUnusable covers the status branches the
// other tests leave out. Swapping EXPIRED and EXHAUSTED would otherwise ship:
// the client would tell a player "no uses left" for a link that merely expired.
func TestGetInviteLinkPreviewReportsWhyUnusable(t *testing.T) {
	expired, _ := model.NewInviteLink("s1", "owner", time.Hour, 0, linkNow.Add(-2*time.Hour))
	exhausted, _ := model.NewInviteLink("s1", "owner", 0, 1, linkNow)
	_ = exhausted.Use(linkNow)

	cases := map[string]struct {
		link *model.InviteLink
		want settlementv1.InviteLinkStatus
	}{
		"expired":   {expired, settlementv1.InviteLinkStatus_INVITE_LINK_STATUS_EXPIRED},
		"exhausted": {exhausted, settlementv1.InviteLinkStatus_INVITE_LINK_STATUS_EXHAUSTED},
	}
	for name, c := range cases {
		svc, repo := newInviteService(t)
		repo.getByCode = c.link

		got, err := svc.GetInviteLink(context.Background(), &settlementv1.GetInviteLinkRequest{Code: c.link.Code})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.GetStatus() != c.want {
			t.Errorf("%s: want %v, got %v", name, c.want, got.GetStatus())
		}
	}
}
