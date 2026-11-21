package service

import (
	"context"
	"errors"
	"testing"
	"time"

	mailv1 "github.com/lasthearth/vsservice/gen/mail/v1"
	"github.com/lasthearth/vsservice/internal/mail/internal/model"
	"github.com/lasthearth/vsservice/internal/mail/internal/service/sermapper"
	pkgerr "github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mongox/pagination"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
)

// fakeMailRepo records what the handler asks for and returns canned rows. Only
// ListMailsAdmin is exercised here; the rest satisfy MailRepository.
type fakeMailRepo struct {
	entries []*model.AdminMailEntry
	next    string
	err     error

	gotFilter    model.AdminMailFilter
	gotPageToken string
	gotPageSize  int64
	calls        int
}

func (f *fakeMailRepo) ListMailsAdmin(
	_ context.Context,
	filter model.AdminMailFilter,
	pageToken string,
	pageSize int64,
) ([]*model.AdminMailEntry, string, error) {
	f.calls++
	f.gotFilter, f.gotPageToken, f.gotPageSize = filter, pageToken, pageSize
	return f.entries, f.next, f.err
}

// errFakeNotUsed marks the repository methods this file never exercises, so a
// stub never reads as a successful empty answer.
var errFakeNotUsed = errors.New("fake mail repository: method not used by this test")

func (f *fakeMailRepo) CreateMail(context.Context, *model.Mail) (*model.Mail, error) {
	return nil, errFakeNotUsed
}

func (f *fakeMailRepo) GetMail(context.Context, string) (*model.Mail, error) {
	return nil, errFakeNotUsed
}

func (f *fakeMailRepo) UpdateMail(
	context.Context,
	string,
	func(context.Context, *model.Mail) (*model.Mail, error),
) (*model.Mail, error) {
	return nil, errFakeNotUsed
}

func (f *fakeMailRepo) ListMailsForRecipient(context.Context, string) ([]*model.Mail, error) {
	return nil, errFakeNotUsed
}

func (f *fakeMailRepo) InsertClaim(context.Context, *model.MailClaim) (bool, error) {
	return false, errFakeNotUsed
}

func (f *fakeMailRepo) GetClaim(context.Context, string, string) (*model.MailClaim, error) {
	return nil, errFakeNotUsed
}

func (f *fakeMailRepo) UpdateClaim(
	context.Context,
	string, string,
	func(context.Context, *model.MailClaim) (*model.MailClaim, error),
) (*model.MailClaim, error) {
	return nil, errFakeNotUsed
}

func (f *fakeMailRepo) ListClaimsForPlayer(context.Context, string) ([]*model.MailClaim, error) {
	return nil, errFakeNotUsed
}

func newAdminListService(t *testing.T, repo MailRepository) *Service {
	t.Helper()

	zc := zap.NewProductionConfig()
	zc.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	return New(Opts{Repo: repo, Logger: l, Mapper: &sermapper.MapperImpl{}})
}

// The filters and the page token reach the repository unchanged, and an absent
// page size becomes the list's default rather than zero (zero would make the
// pagination package fall back to its own 25).
func TestAdminListMailPassesFilterAndPageToRepo(t *testing.T) {
	repo := &fakeMailRepo{}
	s := newAdminListService(t, repo)

	if _, err := s.AdminListMail(context.Background(), &mailv1.AdminListMailRequest{
		Recipient:     "user-1",
		BroadcastOnly: true,
		PageToken:     "token-1",
	}); err != nil {
		t.Fatalf("AdminListMail: %v", err)
	}

	if repo.gotFilter.Recipient != "user-1" || !repo.gotFilter.BroadcastOnly {
		t.Fatalf("filter = %+v, want recipient user-1 with broadcast_only", repo.gotFilter)
	}
	if repo.gotPageToken != "token-1" {
		t.Fatalf("page token = %q, want token-1", repo.gotPageToken)
	}
	if repo.gotPageSize != adminMailDefaultPage {
		t.Fatalf("page size = %d, want %d", repo.gotPageSize, adminMailDefaultPage)
	}
}

func TestAdminPageSizeClamps(t *testing.T) {
	cases := []struct {
		name      string
		requested int32
		want      int64
	}{
		{"absent or negative falls back to the default", 0, 20},
		{"negative falls back to the default", -5, 20},
		{"a normal request passes through", 50, 50},
		{"the ceiling holds", 100, 100},
		{"above the ceiling is clamped", 500, 100},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := adminPageSize(c.requested); got != c.want {
				t.Fatalf("adminPageSize(%d) = %d, want %d", c.requested, got, c.want)
			}
		})
	}
}

// The delivery counter is the point of the endpoint: it must survive the trip
// from the read model to the wire, together with the broadcast flag.
func TestAdminListMailMapsCountersAndAttachments(t *testing.T) {
	expires := time.Now().Add(time.Hour)
	mail := model.ReconstituteMail(
		"mail-1", model.RecipientBroadcast, "admin:sub-1", "Season reward", "Everyone gets coins",
		[]model.Attachment{
			{Coins: &model.CoinsAttachment{Amount: 3000}},
			{Item: &model.ItemAttachment{GameCode: "game:bread", Quantity: 5, Type: "item"}},
		},
		time.Now().UTC().Truncate(time.Second), &expires, true, "",
	)

	repo := &fakeMailRepo{
		entries: []*model.AdminMailEntry{model.NewAdminMailEntry(mail, 42)},
		next:    "next-1",
	}
	s := newAdminListService(t, repo)

	resp, err := s.AdminListMail(context.Background(), &mailv1.AdminListMailRequest{})
	if err != nil {
		t.Fatalf("AdminListMail: %v", err)
	}
	if len(resp.GetMails()) != 1 {
		t.Fatalf("mails = %d, want 1", len(resp.GetMails()))
	}

	got := resp.GetMails()[0]
	if got.GetClaimedCount() != 42 {
		t.Fatalf("claimed_count = %d, want 42", got.GetClaimedCount())
	}
	if !got.GetBroadcast() {
		t.Fatal("broadcast = false, want true for a broadcast mail")
	}
	if got.GetRevoked() != true || got.GetExpiresAt() == nil {
		t.Fatalf("revoked = %v, expires_at = %v, want both carried through", got.GetRevoked(), got.GetExpiresAt())
	}
	if got.GetSender() != "admin:sub-1" || got.GetTitle() != "Season reward" {
		t.Fatalf("sender/title = %q/%q, want the stored values", got.GetSender(), got.GetTitle())
	}
	if len(got.GetAttachments()) != 2 {
		t.Fatalf("attachments = %d, want 2", len(got.GetAttachments()))
	}
	if got.GetAttachments()[0].GetCoins().GetAmount() != 3000 {
		t.Fatalf("coins = %d, want 3000", got.GetAttachments()[0].GetCoins().GetAmount())
	}
	if got.GetAttachments()[1].GetItem().GetGameCode() != "game:bread" {
		t.Fatalf("item = %+v, want game:bread", got.GetAttachments()[1].GetItem())
	}
	if resp.GetNextPageToken() != "next-1" {
		t.Fatalf("next_page_token = %q, want next-1", resp.GetNextPageToken())
	}
}

// A targeted mail must not read as a broadcast.
func TestAdminListMailTargetedIsNotBroadcast(t *testing.T) {
	mail := model.ReconstituteMail(
		"mail-2", "user-9", "admin:sub-1", "Compensation", "Sorry",
		nil, time.Now(), nil, false, "",
	)

	s := newAdminListService(t, &fakeMailRepo{
		entries: []*model.AdminMailEntry{model.NewAdminMailEntry(mail, 0)},
	})

	resp, err := s.AdminListMail(context.Background(), &mailv1.AdminListMailRequest{})
	if err != nil {
		t.Fatalf("AdminListMail: %v", err)
	}
	if got := resp.GetMails()[0]; got.GetBroadcast() || got.GetClaimedCount() != 0 {
		t.Fatalf("mail = %+v, want a targeted mail with no claims", got)
	}
}

// A page token the pagination package did not mint is the client's mistake, so
// the handler must hand it to the interceptor as-is instead of rewriting it to
// Internal.
func TestAdminListMailKeepsInvalidPageToken(t *testing.T) {
	s := newAdminListService(t, &fakeMailRepo{err: pagination.ErrInvalidPageToken})

	_, err := s.AdminListMail(context.Background(), &mailv1.AdminListMailRequest{PageToken: "garbage"})
	if err == nil {
		t.Fatal("AdminListMail: got nil error, want InvalidArgument")
	}

	var de *pkgerr.DomainError
	if !errors.As(err, &de) {
		t.Fatalf("error = %v, want a DomainError", err)
	}
	if de.Code != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", de.Code)
	}
}

// A plain storage failure stays opaque to the client.
func TestAdminListMailHidesStorageFailure(t *testing.T) {
	s := newAdminListService(t, &fakeMailRepo{err: errors.New("mongo is down")})

	_, err := s.AdminListMail(context.Background(), &mailv1.AdminListMailRequest{})

	var de *pkgerr.DomainError
	if !errors.As(err, &de) {
		t.Fatalf("error = %v, want a DomainError", err)
	}
	if de.Code != codes.Internal {
		t.Fatalf("code = %v, want Internal", de.Code)
	}
}
