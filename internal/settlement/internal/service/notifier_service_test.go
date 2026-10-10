package service_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
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

// fakeNotices records site notifications. The upgrade path notifies members
// concurrently, so the recorder guards its own state and counts how many calls
// were in flight at once.
type fakeNotices struct {
	mu       sync.Mutex
	users    []string
	inFlight int
	peak     int
	err      error
}

func (f *fakeNotices) NotifyUser(_ context.Context, userId, _, _ string) error {
	f.mu.Lock()
	f.inFlight++
	if f.inFlight > f.peak {
		f.peak = f.inFlight
	}
	f.mu.Unlock()

	// Hold the slot long enough that an unbounded fan-out is observable.
	time.Sleep(2 * time.Millisecond)

	f.mu.Lock()
	f.inFlight--
	f.users = append(f.users, userId)
	err := f.err
	f.mu.Unlock()
	return err
}

// notified returns the recorded recipients, in arrival order.
func (f *fakeNotices) notified() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.users)
}

// peakConcurrency returns the largest number of notifications that ran at once.
func (f *fakeNotices) peakConcurrency() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.peak
}

// notifierRepo fakes the slice of the repository the approval, submit and
// reissue handlers use. Embedding the interface makes any other call panic.
type notifierRepo struct {
	service.SettlementRepository

	// stored state
	set       *model.Settlement
	request   *model.SettlementVerification
	placement *model.NotifierPlacement
	// others holds further settlements by id, for the handlers that answer with
	// more than one settlement (TransferImperialFavor).
	others map[string]*model.Settlement
	// notifierErr makes GetNotifier fail, to pin the read-failure policy.
	notifierErr error
	// notifierReads counts the calls, to pin which Submit paths pay for one.
	notifierReads int

	// approval outcome
	approval    *service.ApprovalResult
	approved    bool
	updatedReq  *service.SettlementOpts
	createdReq  *service.SettlementOpts
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
	r.notifierReads++
	if r.notifierErr != nil {
		return model.NotifierPlacement{}, false, r.notifierErr
	}
	if r.placement == nil {
		return model.NotifierPlacement{}, false, nil
	}
	return *r.placement, true, nil
}

// lookup finds a stored settlement by id: the primary one first, then others.
func (r *notifierRepo) lookup(id string) *model.Settlement {
	if r.set != nil && r.set.Id == id {
		return r.set
	}
	if s, ok := r.others[id]; ok {
		return s
	}
	return nil
}

func (r *notifierRepo) GetSettlement(_ context.Context, id string) (*model.Settlement, error) {
	if s := r.lookup(id); s != nil {
		return s, nil
	}
	return nil, ierror.ErrNotFound
}

func (r *notifierRepo) GetSettlementByUserId(_ context.Context, userId string) (*model.Settlement, error) {
	if r.set != nil && slices.Contains(r.set.MemberIds(), userId) {
		return r.set, nil
	}
	for _, s := range r.others {
		if slices.Contains(s.MemberIds(), userId) {
			return s, nil
		}
	}
	return nil, ierror.ErrNotFound
}

func (r *notifierRepo) UpdateSettlement(
	ctx context.Context, id string,
	fn func(context.Context, *model.Settlement) (*model.Settlement, error),
) (*model.Settlement, error) {
	cur := r.lookup(id)
	if cur == nil {
		return nil, ierror.ErrNotFound
	}
	working := *cur
	updated, err := fn(ctx, &working)
	if err != nil {
		return nil, err
	}
	if r.set != nil && r.set.Id == id {
		r.set = updated
	} else {
		r.others[id] = updated
	}
	return updated, nil
}

func (r *notifierRepo) IsLeaderOfSettlement(_ context.Context, settlementID, userID string) error {
	s := r.lookup(settlementID)
	if s == nil || !s.IsOwner(userID) {
		return ierror.ErrNotLeader
	}
	return nil
}

func (r *notifierRepo) AddTag(context.Context, string, string) error    { return nil }
func (r *notifierRepo) RemoveTag(context.Context, string, string) error { return nil }

func (r *notifierRepo) CreateFavorLog(context.Context, model.ImperialFavorLog) error { return nil }

func (r *notifierRepo) IsMemberOfAnySettlement(context.Context, string) error { return nil }

func (r *notifierRepo) GetSettlementRequestByLeader(context.Context, string) (*model.SettlementVerification, error) {
	if r.request == nil {
		return nil, ierror.ErrNotFound
	}
	return r.request, nil
}

func (r *notifierRepo) CreateRequest(_ context.Context, opts service.SettlementOpts) error {
	r.createdReq = &opts
	return nil
}

// GetAllSettlements serves List, which deliberately leaves notifier_position unset.
func (r *notifierRepo) GetAllSettlements(context.Context) ([]model.Settlement, error) {
	out := make([]model.Settlement, 0, len(r.others)+1)
	if r.set != nil {
		out = append(out, *r.set)
	}
	for _, s := range r.others {
		out = append(out, *s)
	}
	return out, nil
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
	if len(notices.notified()) != 0 {
		t.Errorf("creation must not send site notifications, got %v", notices.notified())
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

	// The notifications go out concurrently, so only the set is asserted.
	notified := notices.notified()
	slices.Sort(notified)
	if want := []string{"builder", "owner1", "owner2", "resident"}; !slices.Equal(notified, want) {
		t.Errorf("site notifications to %v, want every member %v", notified, want)
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
	if len(mail.sent) != 0 || len(notices.notified()) != 0 {
		t.Errorf("mails %+v, notices %v; want none for an edit that keeps the tier", mail.sent, notices.notified())
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

// An owner without a user id has no mailbox. The upgrade-notice path had no
// guard, so it composed a mail addressed to nobody while Approve reported
// success. sendNotifierMail already refused the same input.
func TestApproveUpgradeRefusesAnOwnerWithoutUserId(t *testing.T) {
	set := &model.Settlement{
		Id:      "s1",
		Name:    "Северный Оплот",
		Type:    model.SettlementTypeVillage,
		Members: []model.Member{{UserId: "", RoleIds: []string{model.OwnerRoleId}}},
	}
	repo := &notifierRepo{
		request: &model.SettlementVerification{Id: "s1", Status: model.SettlementStatusPending},
		approval: &service.ApprovalResult{
			Settlement:   *set,
			PreviousType: model.SettlementTypeCamp,
			RequestedAt:  submittedAt,
		},
	}
	svc, mail, _ := newNotifierService(t, repo)

	_, err := svc.Approve(context.Background(), &settlementv1.ApproveRequest{Id: "s1"})
	if !errors.Is(err, ierror.ErrNoOwnerToDeliver) {
		t.Fatalf("want ErrNoOwnerToDeliver, got %v", err)
	}
	if len(mail.sent) != 0 {
		t.Errorf("mails = %+v, want none", mail.sent)
	}
	if repo.txCommitted {
		t.Error("transaction committed although the notice had no recipient")
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
	_, err := svc.Submit(asUser("owner1"), submitRequest(x, y))
	if err != nil {
		t.Fatal(err)
	}
}

// submitRequest is one upgrade submission with the given coordinates.
func submitRequest(x, y int32) *settlementv1.SubmitRequest {
	return &settlementv1.SubmitRequest{
		Type:        settlementv1.SubmitRequest_CAMP,
		Name:        "Северный Оплот",
		Coordinates: &settlementv1.Vector2{X: x, Y: y},
		Attachments: []*settlementv1.SubmitRequest_SubmitAttachment{{Url: "https://cdn.test/a.png", Description: "d"}},
	}
}

// The notifier read answers one question: whether a block stands for this
// request's settlement. A pending request is refused outright and a first-time
// rejected one has no settlement behind it, so neither may pay for a query whose
// result is thrown away or guaranteed empty.
func TestSubmitSkipsTheNotifierReadWhenItCannotAnswer(t *testing.T) {
	placement := &model.NotifierPlacement{SettlementId: "s1", Position: model.Vector3{X: 100, Y: 64, Z: -300}}

	cases := []struct {
		status    model.SettlementStatus
		wantReads int
		wantErr   bool
	}{
		{model.SettlementStatusPending, 0, true},
		{model.SettlementStatusRejected, 0, false},
		{model.SettlementStatusApproved, 1, false},
		{model.SettlementStatusUpdateRejected, 1, false},
	}

	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			repo := &notifierRepo{
				request: &model.SettlementVerification{
					Id: "s1", Status: tc.status, Type: model.SettlementTypeCamp, UpdatedAt: submittedAt,
				},
				placement: placement,
			}
			svc, _, _ := newNotifierService(t, repo)

			res, err := svc.Submit(asUser("owner1"), submitRequest(10, 20))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("status %s: want AlreadyExists, got %+v", tc.status, res)
				}
			} else if err != nil {
				t.Fatalf("status %s: %v", tc.status, err)
			}
			if repo.notifierReads != tc.wantReads {
				t.Errorf("status %s: notifier reads = %d, want %d", tc.status, repo.notifierReads, tc.wantReads)
			}
		})
	}
}

// A rejected first-time request keeps the coordinates the player typed: there is
// no settlement, so there is no block to override them with.
func TestSubmitRejectedRequestKeepsTypedCoordinates(t *testing.T) {
	repo := &notifierRepo{request: &model.SettlementVerification{
		Id: "s1", Status: model.SettlementStatusRejected, Type: model.SettlementTypeCamp, UpdatedAt: submittedAt,
	}}
	svc, _, _ := newNotifierService(t, repo)

	res, err := svc.Submit(asUser("owner1"), submitRequest(10, 20))
	if err != nil {
		t.Fatal(err)
	}
	if res.GetCoordinatesLocked() {
		t.Error("coordinates_locked = true, want false for a request that was never approved")
	}
	if got := repo.updatedReq.Coordinates; got != (model.Vector2{X: 10, Y: 20}) {
		t.Errorf("stored coordinates = %+v, want the typed (10,20)", got)
	}
}

// The upgrade notifications used to be one sequential database write per member
// on the Approve response path, where the admin waits for the answer. They are
// bounded and concurrent now, and every member is still told.
func TestApproveUpgradeNotifiesMembersWithBoundedConcurrency(t *testing.T) {
	set := memberSet(model.SettlementTypeVillage)
	for i := range 40 {
		set.Members = append(set.Members, model.Member{UserId: fmt.Sprintf("m%02d", i), RoleIds: []string{}})
	}
	repo := &notifierRepo{
		request: &model.SettlementVerification{Id: "s1", Status: model.SettlementStatusPending},
		approval: &service.ApprovalResult{
			Settlement: *set, PreviousType: model.SettlementTypeCamp, RequestedAt: submittedAt,
		},
	}
	svc, _, notices := newNotifierService(t, repo)

	if _, err := svc.Approve(context.Background(), &settlementv1.ApproveRequest{Id: "s1"}); err != nil {
		t.Fatal(err)
	}

	if got := len(notices.notified()); got != len(set.Members) {
		t.Errorf("notified %d members, want all %d", got, len(set.Members))
	}
	peak := notices.peakConcurrency()
	if peak > 8 {
		t.Errorf("peak concurrency = %d, want at most 8", peak)
	}
	if peak < 2 {
		t.Errorf("peak concurrency = %d, want the members notified concurrently", peak)
	}
}

// The approval is committed before the notifications go out, so a failing
// notifier must not undo it and must not fail the response.
func TestApproveUpgradeSucceedsWhenANotificationFails(t *testing.T) {
	set := memberSet(model.SettlementTypeVillage)
	repo := &notifierRepo{
		request: &model.SettlementVerification{Id: "s1", Status: model.SettlementStatusPending},
		approval: &service.ApprovalResult{
			Settlement: *set, PreviousType: model.SettlementTypeCamp, RequestedAt: submittedAt,
		},
	}
	svc, _, notices := newNotifierService(t, repo)
	notices.err = errors.New("notification store down")

	if _, err := svc.Approve(context.Background(), &settlementv1.ApproveRequest{Id: "s1"}); err != nil {
		t.Fatalf("want the approval to succeed, got %v", err)
	}
	if got := len(notices.notified()); got != len(set.Members) {
		t.Errorf("attempted %d notifications, want every member (%d)", got, len(set.Members))
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
