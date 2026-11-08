package service

import (
	"context"
	"errors"
	"testing"
	"time"

	ticketv1 "github.com/lasthearth/vsservice/gen/ticket/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/ticket/internal/ierror"
	"github.com/lasthearth/vsservice/internal/ticket/internal/model"
	"github.com/lasthearth/vsservice/internal/ticket/internal/repository/repomapper"
	"go.uber.org/zap/zapcore"
)

// fakeRepo keeps tickets in memory; Update applies fn to a copy, like the
// real read-modify-replace cycle.
type fakeRepo struct {
	tickets map[string]*model.Ticket
}

func newFakeRepo() *fakeRepo { return &fakeRepo{tickets: map[string]*model.Ticket{}} }

// mapper round-trips a ticket through its document, the way the database does:
// a deep copy, and the only way to give it an id outside the model package.
var mapper repomapper.MapperImpl

func clone(t *model.Ticket) *model.Ticket {
	doc := mapper.FromModel(*t)
	doc.Id, _ = mongox.ParseObjectID(t.Id)
	doc.CreatedAt = t.CreatedAt
	c := mapper.ToModel(doc)
	return &c
}

func (f *fakeRepo) Create(_ context.Context, t *model.Ticket) (*model.Ticket, error) {
	doc := mapper.FromModel(*t)
	doc.Model = mongox.NewModel()
	c := mapper.ToModel(doc)
	f.tickets[c.Id] = &c
	return clone(&c), nil
}

func (f *fakeRepo) Get(_ context.Context, id string) (*model.Ticket, error) {
	t, ok := f.tickets[id]
	if !ok {
		return nil, ierror.ErrNotFound
	}
	return clone(t), nil
}

func (f *fakeRepo) Update(ctx context.Context, id string, fn func(context.Context, *model.Ticket) (*model.Ticket, error)) (*model.Ticket, error) {
	t, ok := f.tickets[id]
	if !ok {
		return nil, ierror.ErrNotFound
	}
	out, err := fn(ctx, clone(t))
	if err != nil {
		return nil, err
	}
	f.tickets[id] = clone(out)
	return out, nil
}

func (f *fakeRepo) List(_ context.Context, q model.Query) ([]model.Ticket, string, error) {
	var out []model.Ticket
	for _, t := range f.tickets {
		if q.AuthorId != "" && t.AuthorId != q.AuthorId {
			continue
		}
		out = append(out, *clone(t))
	}
	return out, "", nil
}

func (f *fakeRepo) CountActiveByAuthor(_ context.Context, authorId string) (int64, error) {
	var n int64
	for _, t := range f.tickets {
		if t.AuthorId == authorId && !t.IsClosed() {
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) CountUnreadByAuthor(_ context.Context, authorId string) (int64, error) {
	var n int64
	for _, t := range f.tickets {
		if t.AuthorId == authorId && t.AuthorUnread {
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) StaffCounters(context.Context, string) (model.StaffCounters, error) {
	return model.StaffCounters{}, nil
}

func (f *fakeRepo) ListAutoCloseDue(_ context.Context, cutoff time.Time, _ int) ([]string, error) {
	var ids []string
	for id, t := range f.tickets {
		if t.Status == model.StatusAnswered && t.AnsweredAt != nil && !t.AnsweredAt.After(cutoff) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (f *fakeRepo) ListReplies(context.Context) ([]model.SavedReply, error) { return nil, nil }
func (f *fakeRepo) CountReplies(context.Context) (int64, error)             { return 0, nil }
func (f *fakeRepo) CreateReply(_ context.Context, r *model.SavedReply) (*model.SavedReply, error) {
	return r, nil
}

func (f *fakeRepo) UpdateReply(ctx context.Context, _ string, fn func(context.Context, *model.SavedReply) (*model.SavedReply, error)) (*model.SavedReply, error) {
	return nil, ierror.ErrReplyNotFound
}
func (f *fakeRepo) DeleteReply(context.Context, string) error { return nil }

type fakeNotifier struct {
	opened, authorWrote, staffReplied []string
}

func (n *fakeNotifier) TicketOpened(_ context.Context, t *model.Ticket) {
	n.opened = append(n.opened, t.Id)
}

func (n *fakeNotifier) AuthorWrote(_ context.Context, t *model.Ticket, _ string) {
	n.authorWrote = append(n.authorWrote, t.Id)
}

func (n *fakeNotifier) StaffReplied(_ context.Context, t *model.Ticket) {
	n.staffReplied = append(n.staffReplied, t.Id)
}

func newTestService() (*Service, *fakeRepo, *fakeNotifier) {
	repo, n := newFakeRepo(), &fakeNotifier{}
	s := &Service{
		logger:   logger.NewWithCore(zapcore.NewNopCore()),
		repo:     repo,
		notifier: n,
		now:      func() time.Time { return time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC) },
	}
	return s, repo, n
}

func as(uid string) context.Context {
	return interceptor.ContextWithUserID(context.Background(), uid)
}

func create(t *testing.T, s *Service, uid string) *ticketv1.Ticket {
	t.Helper()
	tk, err := s.CreateTicket(as(uid), &ticketv1.CreateTicketRequest{
		Subject:  "Пропал сундук",
		Category: ticketv1.TicketCategory_TICKET_CATEGORY_BUG,
		Text:     "Вчера был",
	})
	if err != nil {
		t.Fatal(err)
	}
	return tk
}

func TestCreateNotifiesStaffAndLimitsActiveTickets(t *testing.T) {
	s, _, n := newTestService()
	for range model.MaxActivePerAuthor {
		create(t, s, "p1")
	}
	_, err := s.CreateTicket(as("p1"), &ticketv1.CreateTicketRequest{
		Subject: "Ещё", Category: ticketv1.TicketCategory_TICKET_CATEGORY_OTHER, Text: "x",
	})
	if !errors.Is(err, ierror.ErrActiveTicketLimit) {
		t.Fatalf("err = %v", err)
	}
	if len(n.opened) != model.MaxActivePerAuthor {
		t.Fatalf("opened alerts = %d", len(n.opened))
	}
}

func TestCreateRejectsUnsetCategory(t *testing.T) {
	s, _, _ := newTestService()
	_, err := s.CreateTicket(as("p1"), &ticketv1.CreateTicketRequest{Subject: "Тема", Text: "x"})
	if !errors.Is(err, ierror.ErrCategoryInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestStrangerSeesNotFound(t *testing.T) {
	s, _, _ := newTestService()
	tk := create(t, s, "p1")

	if _, err := s.GetTicket(as("p2"), &ticketv1.GetTicketRequest{Id: tk.GetId()}); !errors.Is(err, ierror.ErrNotFound) {
		t.Fatalf("get: %v", err)
	}
	_, err := s.PostTicketMessage(as("p2"), &ticketv1.PostTicketMessageRequest{Id: tk.GetId(), Text: "я"})
	if !errors.Is(err, ierror.ErrNotFound) {
		t.Fatalf("post: %v", err)
	}
	if _, err := s.CloseTicket(as("p2"), &ticketv1.CloseTicketRequest{Id: tk.GetId()}); !errors.Is(err, ierror.ErrNotFound) {
		t.Fatalf("close: %v", err)
	}
}

func TestInternalNoteHiddenFromAuthorAndNotNotified(t *testing.T) {
	s, _, n := newTestService()
	tk := create(t, s, "p1")

	if _, err := s.AdminReplyTicket(as("admin"), &ticketv1.AdminReplyTicketRequest{
		Id: tk.GetId(), Text: "Похоже на дюп", Internal: true,
	}); err != nil {
		t.Fatal(err)
	}
	if len(n.staffReplied) != 0 {
		t.Fatal("a note must not notify the author")
	}

	got, err := s.GetTicket(as("p1"), &ticketv1.GetTicketRequest{Id: tk.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetMessages()) != 1 || got.GetMessageCount() != 1 {
		t.Fatalf("author sees %d messages", len(got.GetMessages()))
	}

	staff, err := s.AdminGetTicket(as("admin"), &ticketv1.AdminGetTicketRequest{Id: tk.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if len(staff.GetMessages()) != 2 || !staff.GetMessages()[1].GetInternal() {
		t.Fatal("staff sees the note")
	}
}

func TestReplyFlowAndUnreadMarks(t *testing.T) {
	s, repo, n := newTestService()
	tk := create(t, s, "p1")

	reply, err := s.AdminReplyTicket(as("admin"), &ticketv1.AdminReplyTicketRequest{Id: tk.GetId(), Text: "Проверим"})
	if err != nil {
		t.Fatal(err)
	}
	if reply.GetStatus() != ticketv1.TicketStatus_TICKET_STATUS_ANSWERED || reply.GetAssigneeId() != "admin" {
		t.Fatalf("status %s assignee %q", reply.GetStatus(), reply.GetAssigneeId())
	}
	if len(n.staffReplied) != 1 {
		t.Fatal("the author is told about the reply")
	}

	c, _ := s.GetMyTicketCounters(as("p1"), &ticketv1.GetMyTicketCountersRequest{})
	if c.GetUnread() != 1 {
		t.Fatalf("unread = %d", c.GetUnread())
	}
	if _, err := s.GetTicket(as("p1"), &ticketv1.GetTicketRequest{Id: tk.GetId()}); err != nil {
		t.Fatal(err)
	}
	if repo.tickets[tk.GetId()].AuthorUnread {
		t.Fatal("reading clears the author's mark")
	}

	back, err := s.PostTicketMessage(as("p1"), &ticketv1.PostTicketMessageRequest{Id: tk.GetId(), Text: "Медный"})
	if err != nil {
		t.Fatal(err)
	}
	if back.GetStatus() != ticketv1.TicketStatus_TICKET_STATUS_OPEN || len(n.authorWrote) != 1 {
		t.Fatalf("status %s alerts %d", back.GetStatus(), len(n.authorWrote))
	}
}

func TestReopenRespectsTheLimit(t *testing.T) {
	s, _, _ := newTestService()
	first := create(t, s, "p1")
	if _, err := s.CloseTicket(as("p1"), &ticketv1.CloseTicketRequest{Id: first.GetId()}); err != nil {
		t.Fatal(err)
	}
	for range model.MaxActivePerAuthor {
		create(t, s, "p1")
	}
	_, err := s.ReopenTicket(as("p1"), &ticketv1.ReopenTicketRequest{Id: first.GetId()})
	if !errors.Is(err, ierror.ErrActiveTicketLimit) {
		t.Fatalf("err = %v", err)
	}
}

func TestAdminUpdateTicket(t *testing.T) {
	s, _, _ := newTestService()
	tk := create(t, s, "p1")

	cat := ticketv1.TicketCategory_TICKET_CATEGORY_DONATE
	who := "admin2"
	got, err := s.AdminUpdateTicket(as("admin"), &ticketv1.AdminUpdateTicketRequest{
		Id: tk.GetId(), Category: &cat, AssigneeId: &who,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetCategory() != cat || got.GetAssigneeId() != who || got.GetStatus() != ticketv1.TicketStatus_TICKET_STATUS_OPEN {
		t.Fatalf("got %+v", got)
	}

	bad := ticketv1.TicketStatus_TICKET_STATUS_UNSPECIFIED
	_, err = s.AdminUpdateTicket(as("admin"), &ticketv1.AdminUpdateTicketRequest{Id: tk.GetId(), Status: &bad})
	if !errors.Is(err, ierror.ErrStatusInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestCloseStale(t *testing.T) {
	s, repo, _ := newTestService()
	tk := create(t, s, "p1")
	if _, err := s.AdminReplyTicket(as("admin"), &ticketv1.AdminReplyTicketRequest{Id: tk.GetId(), Text: "ok"}); err != nil {
		t.Fatal(err)
	}

	s.CloseStale(context.Background(), s.now().Add(model.AutoCloseAfter-time.Hour))
	if repo.tickets[tk.GetId()].IsClosed() {
		t.Fatal("closed too early")
	}
	s.CloseStale(context.Background(), s.now().Add(model.AutoCloseAfter))
	if !repo.tickets[tk.GetId()].IsClosed() {
		t.Fatal("a quiet answered ticket closes itself")
	}
}
