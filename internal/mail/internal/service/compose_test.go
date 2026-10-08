package service

import (
	"context"
	"errors"
	"testing"

	mailerr "github.com/lasthearth/vsservice/internal/mail/internal/ierror"
	"github.com/lasthearth/vsservice/internal/mail/internal/model"
	"github.com/lasthearth/vsservice/internal/mail/mailcompose"
	pkgerr "github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap"
)

// fakeKitReader is a hand-written stand-in for KitReader.
type fakeKitReader struct {
	snap *KitSnapshot
	err  error
}

func (f fakeKitReader) GetKit(context.Context, string) (*KitSnapshot, error) {
	return f.snap, f.err
}

func TestExpandKitMapsItemsThrough(t *testing.T) {
	kits := fakeKitReader{snap: &KitSnapshot{Items: []KitItem{
		{GameCode: "game:bread", Type: "item", Quantity: 3, AttrSnapshot: "abc"},
		{GameCode: "game:log", Type: "block", Quantity: 1},
	}}}

	got, err := expandKit(context.Background(), kits, "foodkit")
	if err != nil {
		t.Fatalf("expandKit: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("attachments = %d, want 2", len(got))
	}
	a := got[0].Item
	if a == nil || a.GameCode != "game:bread" || a.Type != "item" || a.Quantity != 3 || a.AttrSnapshot != "abc" {
		t.Fatalf("attachment 0 = %+v, want game:bread item x3 attr=abc carried through as-is", a)
	}
	if got[1].Item == nil || got[1].Item.Type != "block" {
		t.Fatalf("attachment 1 = %+v, want the block type preserved", got[1].Item)
	}
}

// An empty (but captured) kit is fail-loud: ErrKitEmpty, never a claimless mail.
func TestExpandKitEmptyFailsLoud(t *testing.T) {
	kits := fakeKitReader{snap: &KitSnapshot{Items: nil}}

	_, err := expandKit(context.Background(), kits, "emptykit")
	if !errors.Is(err, mailerr.ErrKitEmpty) {
		t.Fatalf("expandKit: got %v, want ErrKitEmpty", err)
	}
}

// A kit that was never captured is normalized to ErrKitNotFound.
func TestExpandKitNotFound(t *testing.T) {
	kits := fakeKitReader{err: pkgerr.NotFound("kit not found")}

	_, err := expandKit(context.Background(), kits, "nope")
	if !errors.Is(err, mailerr.ErrKitNotFound) {
		t.Fatalf("expandKit: got %v, want ErrKitNotFound", err)
	}
}

// A non-NotFound reader error is passed through, not swallowed as not-found.
func TestExpandKitPassesThroughOtherErrors(t *testing.T) {
	sentinel := errors.New("db down")
	kits := fakeKitReader{err: sentinel}

	_, err := expandKit(context.Background(), kits, "foodkit")
	if !errors.Is(err, sentinel) {
		t.Fatalf("expandKit: got %v, want the raw reader error", err)
	}
}

// ensure model.Attachment is referenced (guards against an unused import if the
// mapping helpers change).
var _ = model.Attachment{}

// recordingRepo captures the mail handed to CreateMail.
type recordingRepo struct {
	MailRepository
	created []*model.Mail
}

func (r *recordingRepo) CreateMail(_ context.Context, m *model.Mail) (*model.Mail, error) {
	r.created = append(r.created, m)
	return m, nil
}

func newTestComposer(t *testing.T) (*Composer, *recordingRepo) {
	t.Helper()
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}
	repo := &recordingRepo{}
	return &Composer{repo: repo, log: l}, repo
}

// A notification mail carries no attachments, so it is never claimable.
func TestComposeNotificationMailHasNoAttachments(t *testing.T) {
	c, repo := newTestComposer(t)

	err := c.ComposeNotificationMail(context.Background(), mailcompose.SenderSettlement, "u1", "title", "body", "key-1")
	if err != nil {
		t.Fatalf("ComposeNotificationMail: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("created = %d mails, want 1", len(repo.created))
	}
	m := repo.created[0]
	if m.HasAttachments() {
		t.Fatalf("attachments = %+v, want none", m.Attachments)
	}
	if m.Recipient != "u1" || m.Sender != mailcompose.SenderSettlement || m.IdempotencyKey != "key-1" || m.ExpiresAt != nil {
		t.Fatalf("mail = %+v, want recipient u1, sender system:settlement, key key-1, no expiry", m)
	}
}

// A system item mail keeps the caller's sender and key and maps the item.
func TestComposeSystemItemMail(t *testing.T) {
	c, repo := newTestComposer(t)

	err := c.ComposeSystemItemMail(context.Background(), mailcompose.SenderSettlement, "u1", "t", "b", "settlement-notifier:s1",
		[]mailcompose.ItemSpec{{GameCode: "lhgui:notifier-camp", Quantity: 1, Type: "block"}})
	if err != nil {
		t.Fatalf("ComposeSystemItemMail: %v", err)
	}
	m := repo.created[0]
	if m.Sender != mailcompose.SenderSettlement || m.IdempotencyKey != "settlement-notifier:s1" {
		t.Fatalf("mail = %+v, want settlement sender and the given key", m)
	}
	if len(m.Attachments) != 1 || m.Attachments[0].Item == nil ||
		m.Attachments[0].Item.GameCode != "lhgui:notifier-camp" || m.Attachments[0].Item.Type != "block" {
		t.Fatalf("attachments = %+v, want one lhgui:notifier-camp block", m.Attachments)
	}
}