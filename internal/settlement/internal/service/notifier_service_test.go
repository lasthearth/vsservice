package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/mail/mailcompose"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mediaurl"
	"github.com/lasthearth/vsservice/internal/settlement/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement/internal/service"
	"github.com/lasthearth/vsservice/internal/settlement/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.uber.org/zap"
)

// sentMail is one mail the fake composer was asked to write.
type sentMail struct {
	sender, recipient, key string
	items                  []mailcompose.ItemSpec
	inTx                   bool
}

// fakeMail records composed mails. Like the real composer it is idempotent on
// the key.
type fakeMail struct {
	mailcompose.MailComposer
	sent []sentMail
	err  error
}

type txKey struct{}

// submittedAt is when the request under test was last submitted. The mail keys
// carry it, so it is what tells one life of a settlement id from the next.
var submittedAt = time.UnixMilli(1700000000000)

func (f *fakeMail) add(ctx context.Context, m sentMail) error {
	if f.err != nil {
		return f.err
	}
	for _, s := range f.sent {
		if s.key == m.key {
			return nil
		}
	}
	m.inTx, _ = ctx.Value(txKey{}).(bool)
	f.sent = append(f.sent, m)
	return nil
}

func (f *fakeMail) ComposeSystemItemMail(ctx context.Context, sender, recipient, _, _, key string, items []mailcompose.ItemSpec) error {
	return f.add(ctx, sentMail{sender: sender, recipient: recipient, key: key, items: items})
}

func (f *fakeMail) ComposeNotificationMail(ctx context.Context, sender, recipient, _, _, key string) error {
	return f.add(ctx, sentMail{sender: sender, recipient: recipient, key: key})
}

func (f *fakeMail) recipients() []string {
	var out []string
	for _, m := range f.sent {
		out = append(out, m.recipient)
	}
	slices.Sort(out)
	return out
}

// fakeNotices records site notifications.
type fakeNotices struct{ users []string }

func (f *fakeNotices) NotifyUser(_ context.Context, userId, _, _ string) error {
	f.users = append(f.users, userId)
	return nil
}

// notifierRepo fakes the slice of the repository the approval, submit and
// reissue handlers use. Embedding the interface makes any other call panic.
type notifierRepo struct {
	service.SettlementRepository

	// stored state
	set       *model.Settlement
	request   *model.SettlementVerification
	placement *model.NotifierPlacement

	// approval outcome
	approval    *service.ApprovalResult
	approved    bool
	updatedReq  *service.SettlementOpts
	txCommitted bool

	// reissue counter mirrored onto the request
	mirroredReissues int
	mirrorCalls      int
	mirrorInTx       bool
}

func (r *notifierRepo) InTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if err := fn(context.WithValue(ctx, txKey{}, true)); err != nil {
		return err
	}
	r.txCommitted = true
	return nil
}

func (r *notifierRepo) GetSettlementRequest(context.Context, string) (*model.SettlementVerification, error) {
	if r.request == nil {
		return nil, ierror.ErrNotFound
	}
	return r.request, nil
}

// SetRequestNotifierReissues records the counter copy the reissue mirrors onto
// the request document.
func (r *notifierRepo) SetRequestNotifierReissues(ctx context.Context, _ string, reissues int) error {
	r.mirrorCalls++
	r.mirroredReissues = reissues
	r.mirrorInTx, _ = ctx.Value(txKey{}).(bool)
	return nil
}

func (r *notifierRepo) Approve(context.Context, string) (*service.ApprovalResult, error) {
	r.approved = true
	return r.approval, nil
}

func (r *notifierRepo) GetNotifier(context.Context, string) (model.NotifierPlacement, bool, error) {
	if r.placement == nil {
		return model.NotifierPlacement{}, false, nil
	}
	return *r.placement, true, nil
}

func (r *notifierRepo) GetSettlement(_ context.Context, id string) (*model.Settlement, error) {
	if r.set == nil || r.set.Id != id {
		return nil, ierror.ErrNotFound
	}
	return r.set, nil
}

func (r *notifierRepo) UpdateSettlement(
	ctx context.Context, id string,
	fn func(context.Context, *model.Settlement) (*model.Settlement, error),
) (*model.Settlement, error) {
	if r.set == nil || r.set.Id != id {
		return nil, ierror.ErrNotFound
	}
	working := *r.set
	updated, err := fn(ctx, &working)
	if err != nil {
		return nil, err
	}
	r.set = updated
	return updated, nil
}

func (r *notifierRepo) IsMemberOfAnySettlement(context.Context, string) error { return nil }

func (r *notifierRepo) GetSettlementRequestByLeader(context.Context, string) (*model.SettlementVerification, error) {
	return r.request, nil
}

func (r *notifierRepo) UpdateRequest(_ context.Context, opts service.SettlementOpts) error {
	r.updatedReq = &opts
	return nil
}

// memberSet is a settlement with two owners and two plain members.
func memberSet(typ model.SettlementType) *model.Settlement {
	return &model.Settlement{
		Id:     "s1",
		Name:   "Северный Оплот",
		Type:   typ,
		Leader: model.Member{UserId: "owner1", RoleIds: []string{model.OwnerRoleId}},
		Members: []model.Member{
			{UserId: "owner1", RoleIds: []string{model.OwnerRoleId}},
			{UserId: "owner2", RoleIds: []string{model.OwnerRoleId}},
			{UserId: "builder", RoleIds: []string{"builders"}},
			{UserId: "resident", RoleIds: []string{}},
		},
	}
}

// newSet is a settlement that has only its founder, as a creation leaves it.
func newSet(typ model.SettlementType) *model.Settlement {
	owner := model.Member{UserId: "owner1", RoleIds: []string{model.OwnerRoleId}}
	return &model.Settlement{Id: "s1", Name: "Северный Оплот", Type: typ, Leader: owner, Members: []model.Member{owner}}
}

func newNotifierService(t *testing.T, repo *notifierRepo) (*service.Service, *fakeMail, *fakeNotices) {
	t.Helper()
	zc := zap.NewProductionConfig()
	log, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}
	mail := &fakeMail{}
	notices := &fakeNotices{}
	svc := service.New(service.Opts{
		DbRepo:   repo,
		Mapper:   &sermapper.MapperImpl{},
		Log:      log,
		Mail:     mail,
		MediaURL: mediaurl.New(config.Config{CdnUrl: "https://cdn.test"}),
	})
	svc.SetNoticesForTest(notices)
	return svc, mail, notices
}

func TestApproveCreationMailsTheBlockToTheLeaderInTheTransaction(t *testing.T) {
	set := newSet(model.SettlementTypeCamp)
	repo := &notifierRepo{
		request: &model.SettlementVerification{Id: "s1", Status: model.SettlementStatusPending, UpdatedAt: submittedAt},
		approval: &service.ApprovalResult{
			Created: true, Settlement: *set, RequestedAt: submittedAt,
		},
	}
	svc, mail, notices := newNotifierService(t, repo)

	if _, err := svc.Approve(context.Background(), &settlementv1.ApproveRequest{Id: "s1"}); err != nil {
		t.Fatal(err)
	}

	if len(mail.sent) != 1 {
		t.Fatalf("mails = %+v, want exactly the block mail", mail.sent)
	}
	m := mail.sent[0]
	if m.recipient != "owner1" || m.sender != mailcompose.SenderSettlement || m.key != "settlement-notifier:s1:1700000000000" {
		t.Errorf("mail = %+v", m)
	}
	if len(m.items) != 1 || m.items[0].GameCode != "lhgui:notifier-camp" || m.items[0].Type != "block" ||
		m.items[0].Quantity != 1 || m.items[0].AttrSnapshot != "" {
		t.Errorf("attachment = %+v", m.items)
	}
	if !m.inTx || !repo.txCommitted {
		t.Errorf("mail written in transaction = %v, committed = %v; want both true", m.inTx, repo.txCommitted)
	}
	if len(notices.users) != 0 {
		t.Errorf("creation must not send site notifications, got %v", notices.users)
	}
}

func TestApproveUpgradeNotifiesAllMembersAndMailsOnlyOwners(t *testing.T) {
	set := memberSet(model.SettlementTypeVillage)
	repo := &notifierRepo{
		request: &model.SettlementVerification{Id: "s1", Status: model.SettlementStatusPending},
		approval: &service.ApprovalResult{
			Settlement:   *set,
			PreviousType: model.SettlementTypeCamp,
			RequestedAt:  time.UnixMilli(1000),
		},
	}
	svc, mail, notices := newNotifierService(t, repo)

	if _, err := svc.Approve(context.Background(), &settlementv1.ApproveRequest{Id: "s1"}); err != nil {
		t.Fatal(err)
	}

	slices.Sort(notices.users)
	if want := []string{"builder", "owner1", "owner2", "resident"}; !slices.Equal(notices.users, want) {
		t.Errorf("site notifications to %v, want every member %v", notices.users, want)
	}
	if want := []string{"owner1", "owner2"}; !slices.Equal(mail.recipients(), want) {
		t.Errorf("mails to %v, want owners only %v", mail.recipients(), want)
	}
	for _, m := range mail.sent {
		if len(m.items) != 0 {
			t.Errorf("upgrade mail to %s carries attachments %+v", m.recipient, m.items)
		}
		if !m.inTx {
			t.Errorf("upgrade mail to %s was written outside the transaction", m.recipient)
		}
	}
}

func TestApproveWithoutTierChangeSendsNothing(t *testing.T) {
	set := memberSet(model.SettlementTypeVillage)
	repo := &notifierRepo{
		request: &model.SettlementVerification{Id: "s1", Status: model.SettlementStatusPending},
		approval: &service.ApprovalResult{
			Settlement:   *set,
			PreviousType: model.SettlementTypeVillage,
		},
	}
	svc, mail, notices := newNotifierService(t, repo)

	if _, err := svc.Approve(context.Background(), &settlementv1.ApproveRequest{Id: "s1"}); err != nil {
		t.Fatal(err)
	}
	if len(mail.sent) != 0 || len(notices.users) != 0 {
		t.Errorf("mails %+v, notices %v; want none for an edit that keeps the tier", mail.sent, notices.users)
	}
}

// A mail that cannot be written must fail the whole approval: the callback
// returns the error, so the real InTransaction rolls the request and the
// settlement back with it.
func TestApproveFailsWhenTheMailFails(t *testing.T) {
	set := memberSet(model.SettlementTypeCamp)
	repo := &notifierRepo{
		request:  &model.SettlementVerification{Id: "s1", Status: model.SettlementStatusPending},
		approval: &service.ApprovalResult{Created: true, Settlement: *set},
	}
	svc, mail, _ := newNotifierService(t, repo)
	mail.err = errors.New("mongo down")

	if _, err := svc.Approve(context.Background(), &settlementv1.ApproveRequest{Id: "s1"}); err == nil {
		t.Fatal("want the approval to fail with the mail")
	}
	if repo.txCommitted {
		t.Error("transaction committed although the mail failed")
	}
}

func TestSubmitUpgradeKeepsClientCoordinatesWithoutNotifier(t *testing.T) {
	repo := &notifierRepo{request: &model.SettlementVerification{
		Id: "s1", Status: model.SettlementStatusApproved, Type: model.SettlementTypeCamp,
	}}
	svc, _, _ := newNotifierService(t, repo)

	submitAt(t, svc, 10, 20)

	if got := repo.updatedReq.Coordinates; got != (model.Vector2{X: 10, Y: 20}) {
		t.Errorf("coordinates = %+v, want the submitted ones", got)
	}
}

func TestSubmitUpgradeCoordinatesFollowTheStandingNotifier(t *testing.T) {
	repo := &notifierRepo{
		request: &model.SettlementVerification{
			Id: "s1", Status: model.SettlementStatusApproved, Type: model.SettlementTypeCamp,
		},
		placement: &model.NotifierPlacement{SettlementId: "s1", Position: model.Vector3{X: 100, Y: 64, Z: -300}},
	}
	svc, _, _ := newNotifierService(t, repo)

	submitAt(t, svc, 10, 20)

	// World X and Z are the site's X and Y; the client value is dropped.
	if got := repo.updatedReq.Coordinates; got != (model.Vector2{X: 100, Y: -300}) {
		t.Errorf("coordinates = %+v, want the notifier's (100, -300)", got)
	}
	if repo.updatedReq.Type != model.SettlementTypeVillage {
		t.Errorf("type = %q, want the level-up to village", repo.updatedReq.Type)
	}
}

func submitAt(t *testing.T, svc *service.Service, x, y int32) {
	t.Helper()
	_, err := svc.Submit(asUser("owner1"), &settlementv1.SubmitRequest{
		Type:        settlementv1.SubmitRequest_CAMP,
		Name:        "Северный Оплот",
		Coordinates: &settlementv1.Vector2{X: x, Y: y},
		Attachments: []*settlementv1.SubmitRequest_SubmitAttachment{{Url: "https://cdn.test/a.png", Description: "d"}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetExposesTheNotifierPosition(t *testing.T) {
	repo := &notifierRepo{
		set:       memberSet(model.SettlementTypeCamp),
		placement: &model.NotifierPlacement{Position: model.Vector3{X: 1, Y: 2, Z: 3}},
	}
	svc, _, _ := newNotifierService(t, repo)

	res, err := svc.Get(context.Background(), &settlementv1.GetRequest{Id: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	p := res.GetSettlement().GetNotifierPosition()
	if p.GetX() != 1 || p.GetY() != 2 || p.GetZ() != 3 {
		t.Errorf("notifier_position = %v, want (1,2,3)", p)
	}

	repo.placement = nil
	res, err = svc.Get(context.Background(), &settlementv1.GetRequest{Id: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetSettlement().GetNotifierPosition() != nil {
		t.Errorf("notifier_position = %v, want unset", res.GetSettlement().GetNotifierPosition())
	}
}

func TestReissueNotifierNumbersEachMail(t *testing.T) {
	set := memberSet(model.SettlementTypeTownship)
	repo := &notifierRepo{set: set, request: &model.SettlementVerification{Id: "s1", UpdatedAt: submittedAt}}
	svc, mail, _ := newNotifierService(t, repo)

	for want := int32(1); want <= 2; want++ {
		res, err := svc.ReissueNotifier(context.Background(), &settlementv1.ReissueNotifierRequest{SettlementId: "s1"})
		if err != nil {
			t.Fatal(err)
		}
		if res.GetNumber() != want {
			t.Errorf("number = %d, want %d", res.GetNumber(), want)
		}
	}

	if len(mail.sent) != 2 {
		t.Fatalf("mails = %+v, want one per reissue", mail.sent)
	}
	if mail.sent[0].key != "settlement-notifier:s1:1700000000000:1" ||
		mail.sent[1].key != "settlement-notifier:s1:1700000000000:2" {
		t.Errorf("keys = %q, %q", mail.sent[0].key, mail.sent[1].key)
	}
	if mail.sent[1].recipient != "owner1" || mail.sent[1].items[0].GameCode != "lhgui:notifier-town" {
		t.Errorf("mail = %+v", mail.sent[1])
	}
	if repo.set.NotifierReissues != 2 {
		t.Errorf("persisted counter = %d, want 2", repo.set.NotifierReissues)
	}
	// The request keeps a copy, in the same transaction, so a settlement
	// re-created under this id continues at 3 instead of restarting at 1.
	if repo.mirroredReissues != 2 || repo.mirrorCalls != 2 || !repo.mirrorInTx {
		t.Errorf("mirrored counter = %d after %d calls (in tx %v), want 2 after 2 in a transaction",
			repo.mirroredReissues, repo.mirrorCalls, repo.mirrorInTx)
	}
}

// Two lives of one settlement id must not share mail keys: DeleteSettlement
// leaves the mails and the request behind, so a re-created settlement that
// reuses the id would otherwise find the earlier life's mail and hand out
// nothing while Approve reports success.
func TestReissueKeyChangesWithTheSubmission(t *testing.T) {
	set := memberSet(model.SettlementTypeCamp)
	repo := &notifierRepo{set: set, request: &model.SettlementVerification{Id: "s1", UpdatedAt: submittedAt}}
	svc, mail, _ := newNotifierService(t, repo)

	if _, err := svc.ReissueNotifier(context.Background(), &settlementv1.ReissueNotifierRequest{SettlementId: "s1"}); err != nil {
		t.Fatal(err)
	}

	// The settlement is deleted and re-created: a new submission, and the counter
	// the request mirrored keeps the numbering going.
	repo.request.UpdatedAt = time.UnixMilli(1800000000000)
	repo.set.NotifierReissues = repo.mirroredReissues

	if _, err := svc.ReissueNotifier(context.Background(), &settlementv1.ReissueNotifierRequest{SettlementId: "s1"}); err != nil {
		t.Fatal(err)
	}

	if len(mail.sent) != 2 {
		t.Fatalf("mails = %+v, want one per life", mail.sent)
	}
	if mail.sent[0].key == mail.sent[1].key {
		t.Errorf("both lives wrote the same key %q", mail.sent[0].key)
	}
	if mail.sent[1].key != "settlement-notifier:s1:1800000000000:2" {
		t.Errorf("second life key = %q, want the new stamp and the continued number", mail.sent[1].key)
	}
}

func TestReissueNotifierRecipientMustBeOwner(t *testing.T) {
	set := memberSet(model.SettlementTypeCamp)
	repo := &notifierRepo{set: set, request: &model.SettlementVerification{Id: "s1", UpdatedAt: submittedAt}}
	svc, mail, _ := newNotifierService(t, repo)

	_, err := svc.ReissueNotifier(context.Background(), &settlementv1.ReissueNotifierRequest{SettlementId: "s1", UserId: "resident"})
	if !errors.Is(err, ierror.ErrNoOwnerToDeliver) {
		t.Fatalf("plain member: want ErrNoOwnerToDeliver, got %v", err)
	}

	if _, err := svc.ReissueNotifier(context.Background(), &settlementv1.ReissueNotifierRequest{SettlementId: "s1", UserId: "owner2"}); err != nil {
		t.Fatal(err)
	}
	if len(mail.sent) != 1 || mail.sent[0].recipient != "owner2" {
		t.Errorf("mails = %+v, want one to owner2", mail.sent)
	}
}

func TestReissueNotifierRefusesTypesWithoutBlock(t *testing.T) {
	set := memberSet(model.SettlementType("khutor"))
	repo := &notifierRepo{set: set, request: &model.SettlementVerification{Id: "s1", UpdatedAt: submittedAt}}
	svc, mail, _ := newNotifierService(t, repo)

	_, err := svc.ReissueNotifier(context.Background(), &settlementv1.ReissueNotifierRequest{SettlementId: "s1"})
	if !errors.Is(err, ierror.ErrNotifierUnavailable) {
		t.Fatalf("want ErrNotifierUnavailable, got %v", err)
	}
	if len(mail.sent) != 0 {
		t.Errorf("mails = %+v, want none", mail.sent)
	}
}

func TestReissueNotifierIsAdminOnly(t *testing.T) {
	svc, _, _ := newNotifierService(t, &notifierRepo{})
	scope := svc.Scope()
	got, ok := scope["/settlement.v1.SettlementService/ReissueNotifier"]
	if !ok || string(got) != "settlements:manage" {
		t.Fatalf("ReissueNotifier scope = %q (declared %v), want settlements:manage", got, ok)
	}
}
