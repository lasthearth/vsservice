package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	ticketv1 "github.com/lasthearth/vsservice/gen/ticket/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/ticket/internal/ierror"
	"github.com/lasthearth/vsservice/internal/ticket/internal/model"
	"github.com/lasthearth/vsservice/internal/ticket/internal/repository/repomapper"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap/zapcore"
)

// fakeRepo keeps tickets in memory; Update applies fn to a copy, like the
// real read-modify-replace cycle.
type fakeRepo struct {
	tickets map[string]*model.Ticket
	replies map[string]model.SavedReply
	// markReadErr, when set, makes MarkRead fail without touching a ticket.
	markReadErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		tickets: map[string]*model.Ticket{},
		replies: map[string]model.SavedReply{},
	}
}

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

func (f *fakeRepo) MarkRead(_ context.Context, id string, staff bool) error {
	if f.markReadErr != nil {
		return f.markReadErr
	}
	t, ok := f.tickets[id]
	if !ok {
		return ierror.ErrNotFound
	}
	if staff {
		t.MarkReadByStaff()
		return nil
	}
	t.MarkReadByAuthor()
	return nil
}

func (f *fakeRepo) List(_ context.Context, q model.Query) ([]model.Ticket, string, error) {
	var out []model.Ticket
	for _, t := range f.tickets {
		switch q.Viewer {
		case model.ViewerAuthor:
			if q.AuthorId == "" || t.AuthorId != q.AuthorId {
				continue
			}
		case model.ViewerStaff:
			if q.AuthorId != "" && t.AuthorId != q.AuthorId {
				continue
			}
		}
		if q.Status != "" && t.Status != q.Status {
			continue
		}
		if q.Category != "" && t.Category != q.Category {
			continue
		}
		switch q.Assignee {
		case model.AssigneeMine:
			if t.AssigneeId != q.StaffId {
				continue
			}
		case model.AssigneeNobody:
			if t.AssigneeId != "" {
				continue
			}
		case model.AssigneeAny:
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

// ListAutoCloseDue deliberately ignores the cutoff and lists every ANSWERED
// ticket, so model.Ticket.AutoClose inside the update is the only gate that
// decides a close. A fake that applied the cutoff itself would never reach it.
func (f *fakeRepo) ListAutoCloseDue(_ context.Context, _ time.Time, limit int) ([]string, error) {
	ids := make([]string, 0, len(f.tickets))
	for id, t := range f.tickets {
		if t.Status == model.StatusAnswered {
			ids = append(ids, id)
		}
	}
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, nil
}

func (f *fakeRepo) ListReplies(context.Context) ([]model.SavedReply, error) {
	out := make([]model.SavedReply, 0, len(f.replies))
	for _, r := range f.replies {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b model.SavedReply) int { return strings.Compare(a.Title, b.Title) })
	return out, nil
}

func (f *fakeRepo) CountReplies(context.Context) (int64, error) {
	return int64(len(f.replies)), nil
}

func (f *fakeRepo) CreateReply(_ context.Context, r *model.SavedReply) (*model.SavedReply, error) {
	// Through the document, the way the real repository does it: the id comes
	// from the Model envelope, not from a field write on the model.
	doc := mapper.ReplyFromModel(*r)
	doc.Model = mongox.NewModel()
	c := mapper.ReplyToModel(doc)
	f.replies[c.Id] = c
	out := c
	return &out, nil
}

func (f *fakeRepo) UpdateReply(_ context.Context, id string, fn func(context.Context, *model.SavedReply) (*model.SavedReply, error)) (*model.SavedReply, error) {
	r, ok := f.replies[id]
	if !ok {
		return nil, ierror.ErrReplyNotFound
	}
	out, err := fn(context.Background(), &r)
	if err != nil {
		return nil, err
	}
	f.replies[id] = *out
	c := *out
	return &c, nil
}

func (f *fakeRepo) DeleteReply(_ context.Context, id string) error {
	if _, ok := f.replies[id]; !ok {
		return ierror.ErrReplyNotFound
	}
	delete(f.replies, id)
	return nil
}

// wroteAlert is one AuthorWrote signal: the ticket id and the text the
// service handed the notifier.
type wroteAlert struct{ id, text string }

type fakeNotifier struct {
	opened, staffReplied []string
	authorWrote          []wroteAlert
}

func (n *fakeNotifier) TicketOpened(_ context.Context, t *model.Ticket) {
	n.opened = append(n.opened, t.Id)
}

func (n *fakeNotifier) AuthorWrote(_ context.Context, t *model.Ticket, text string) {
	n.authorWrote = append(n.authorWrote, wroteAlert{t.Id, text})
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

// stepClock advances the service clock by one minute per call, so separate
// operations leave separate timestamps.
func stepClock(s *Service) {
	base := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	var n int64
	s.now = func() time.Time {
		n++
		return base.Add(time.Duration(n) * time.Minute)
	}
}

func TestListMyTicketsShowsOnlyTheCallersTickets(t *testing.T) {
	s, repo, _ := newTestService()
	stepClock(s)

	mine := create(t, s, "p1")
	theirs := create(t, s, "p2")

	// An internal note moves last_message_at but not last_public_at and grows
	// the total count but not the public one: the author view must follow the
	// public pair, not the raw fields.
	if _, err := s.AdminReplyTicket(as("admin"), &ticketv1.AdminReplyTicketRequest{
		Id: mine.GetId(), Text: "Похоже на дюп", Internal: true,
	}); err != nil {
		t.Fatal(err)
	}
	stored := repo.tickets[mine.GetId()]
	if stored.LastPublicAt.Equal(stored.LastMessageAt) || stored.PublicCount == stored.MessageCount {
		t.Fatalf("precondition: the note must split the views (%v vs %v, %d vs %d)",
			stored.LastPublicAt, stored.LastMessageAt, stored.PublicCount, stored.MessageCount)
	}

	got, err := s.ListMyTickets(as("p1"), &ticketv1.ListMyTicketsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetTickets()) != 1 || got.GetTickets()[0].GetId() != mine.GetId() {
		t.Fatalf("p1 sees %+v, want only %s", got.GetTickets(), mine.GetId())
	}
	tk := got.GetTickets()[0]
	if tk.GetMessages() != nil {
		t.Fatalf("a list row carries the thread: %+v", tk.GetMessages())
	}
	if tk.GetMessageCount() != stored.PublicCount {
		t.Fatalf("message_count = %d, want the public count %d (total %d)",
			tk.GetMessageCount(), stored.PublicCount, stored.MessageCount)
	}
	if !tk.GetLastMessageAt().AsTime().Equal(stored.LastPublicAt) {
		t.Fatalf("last_message_at = %v, want last_public_at %v (not %v)",
			tk.GetLastMessageAt().AsTime(), stored.LastPublicAt, stored.LastMessageAt)
	}

	other, err := s.ListMyTickets(as("p2"), &ticketv1.ListMyTicketsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(other.GetTickets()) != 1 || other.GetTickets()[0].GetId() != theirs.GetId() {
		t.Fatalf("p2 sees %+v, want only %s", other.GetTickets(), theirs.GetId())
	}
}

func TestAuthorNeverSeesTheAssignee(t *testing.T) {
	s, _, _ := newTestService()
	tk := create(t, s, "p1")

	// A staff reply auto-assigns the ticket to the replying admin.
	if _, err := s.AdminReplyTicket(as("admin"), &ticketv1.AdminReplyTicketRequest{Id: tk.GetId(), Text: "Проверим"}); err != nil {
		t.Fatal(err)
	}

	mine, err := s.GetTicket(as("p1"), &ticketv1.GetTicketRequest{Id: tk.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if mine.GetAssigneeId() != "" {
		t.Fatalf("the author sees the internal admin id %q", mine.GetAssigneeId())
	}

	staff, err := s.AdminGetTicket(as("admin"), &ticketv1.AdminGetTicketRequest{Id: tk.GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if staff.GetAssigneeId() != "admin" {
		t.Fatalf("the staff view lost the assignee: %q", staff.GetAssigneeId())
	}
}

func TestReopenSucceedsAndAlerts(t *testing.T) {
	s, repo, n := newTestService()
	tk := create(t, s, "p1")
	id := tk.GetId()

	if _, err := s.AdminReplyTicket(as("admin"), &ticketv1.AdminReplyTicketRequest{Id: id, Text: "Проверим"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CloseTicket(as("p1"), &ticketv1.CloseTicketRequest{Id: id}); err != nil {
		t.Fatal(err)
	}
	if !repo.tickets[id].AuthorUnread {
		t.Fatal("precondition: the unread staff reply survives the close")
	}

	got, err := s.ReopenTicket(as("p1"), &ticketv1.ReopenTicketRequest{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetStatus() != ticketv1.TicketStatus_TICKET_STATUS_OPEN {
		t.Fatalf("status = %s", got.GetStatus())
	}
	if got.GetClosedAt() != nil {
		t.Fatalf("closed_at = %v, want cleared", got.GetClosedAt())
	}
	if got.GetUnread() {
		t.Fatal("the reopen must clear the author's unread mark in the response")
	}
	stored := repo.tickets[id]
	if stored.AuthorUnread {
		t.Fatal("the reopen must clear the stored author's unread mark")
	}
	if !stored.StaffUnread {
		t.Fatal("the reopen must set the staff unread mark")
	}
	// Exactly one alert, with an empty text: that is the reopen signal.
	if len(n.authorWrote) != 1 || n.authorWrote[0].id != id || n.authorWrote[0].text != "" {
		t.Fatalf("alerts = %+v, want one for %s with an empty text", n.authorWrote, id)
	}
}

func TestReopenRejectsBeforeTheCapCheck(t *testing.T) {
	s, _, _ := newTestService()
	open := create(t, s, "p1")
	for range model.MaxActivePerAuthor - 1 {
		create(t, s, "p1")
	}
	// p1 now sits at the active-ticket cap.

	// State first: the ticket is not closed, so the cap is not the answer.
	_, err := s.ReopenTicket(as("p1"), &ticketv1.ReopenTicketRequest{Id: open.GetId()})
	if !errors.Is(err, ierror.ErrNotClosed) {
		t.Fatalf("an open ticket: err = %v, want ErrNotClosed", err)
	}

	// Ownership first: someone else's ticket looks like a missing one even
	// when the caller is at the cap.
	theirs := create(t, s, "p2")
	if _, err := s.CloseTicket(as("p2"), &ticketv1.CloseTicketRequest{Id: theirs.GetId()}); err != nil {
		t.Fatal(err)
	}
	_, err = s.ReopenTicket(as("p1"), &ticketv1.ReopenTicketRequest{Id: theirs.GetId()})
	if !errors.Is(err, ierror.ErrNotFound) {
		t.Fatalf("a stranger's ticket: err = %v, want ErrNotFound", err)
	}
}

func TestPostMessageNotifiesWithTheStoredText(t *testing.T) {
	s, _, n := newTestService()
	tk := create(t, s, "p1")

	if _, err := s.PostTicketMessage(as("p1"), &ticketv1.PostTicketMessageRequest{
		Id: tk.GetId(), Text: "   padded text   ",
	}); err != nil {
		t.Fatal(err)
	}
	if len(n.authorWrote) != 1 {
		t.Fatalf("alerts = %+v, want exactly one", n.authorWrote)
	}
	if n.authorWrote[0].text != "padded text" {
		t.Fatalf("the notifier got %q, want the trimmed stored text", n.authorWrote[0].text)
	}
}

func TestAdminReplyInternalWithCloseIsRejected(t *testing.T) {
	s, repo, n := newTestService()
	tk := create(t, s, "p1")

	_, err := s.AdminReplyTicket(as("admin"), &ticketv1.AdminReplyTicketRequest{
		Id: tk.GetId(), Text: "закрываю", Internal: true, Close: true,
	})
	if !errors.Is(err, ierror.ErrCloseWithNote) {
		t.Fatalf("err = %v, want ErrCloseWithNote", err)
	}
	if len(n.staffReplied) != 0 {
		t.Fatalf("a rejected reply alerted the author: %+v", n.staffReplied)
	}
	stored := repo.tickets[tk.GetId()]
	if stored.Status != model.StatusOpen || stored.MessageCount != 1 {
		t.Fatalf("the rejected reply changed the ticket: status %s, messages %d", stored.Status, stored.MessageCount)
	}
}

func TestSavedReplyLimit(t *testing.T) {
	s, _, _ := newTestService()

	var first string
	for i := range model.MaxSavedReplies {
		got, err := s.CreateSavedReply(as("admin"), &ticketv1.CreateSavedReplyRequest{
			Title: fmt.Sprintf("Шаблон %03d", i), Text: "Ответ поддержки",
		})
		if err != nil {
			t.Fatalf("reply %d: %v", i, err)
		}
		if i == 0 {
			first = got.GetId()
		}
	}

	_, err := s.CreateSavedReply(as("admin"), &ticketv1.CreateSavedReplyRequest{Title: "Лишний", Text: "x"})
	if !errors.Is(err, ierror.ErrSavedReplyLimit) {
		t.Fatalf("the %dth reply: err = %v, want ErrSavedReplyLimit", model.MaxSavedReplies+1, err)
	}

	updated, err := s.UpdateSavedReply(as("admin"), &ticketv1.UpdateSavedReplyRequest{
		Id: first, Title: "Обновлён", Text: "Новый текст",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.GetTitle() != "Обновлён" || updated.GetText() != "Новый текст" {
		t.Fatalf("update returned %+v", updated)
	}
	_, err = s.UpdateSavedReply(as("admin"), &ticketv1.UpdateSavedReplyRequest{
		Id: bson.NewObjectID().Hex(), Title: "Нет такого", Text: "x",
	})
	if !errors.Is(err, ierror.ErrReplyNotFound) {
		t.Fatalf("update of a missing id: err = %v, want ErrReplyNotFound", err)
	}

	if _, err := s.DeleteSavedReply(as("admin"), &ticketv1.DeleteSavedReplyRequest{Id: first}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = s.DeleteSavedReply(as("admin"), &ticketv1.DeleteSavedReplyRequest{Id: first})
	if !errors.Is(err, ierror.ErrReplyNotFound) {
		t.Fatalf("delete of a missing id: err = %v, want ErrReplyNotFound", err)
	}
}

func TestMarkReadFailureDoesNotFailTheRead(t *testing.T) {
	s, repo, _ := newTestService()
	tk := create(t, s, "p1")
	if _, err := s.AdminReplyTicket(as("admin"), &ticketv1.AdminReplyTicketRequest{Id: tk.GetId(), Text: "Ответ"}); err != nil {
		t.Fatal(err)
	}
	repo.markReadErr = errors.New("mark read failed")

	got, err := s.GetTicket(as("p1"), &ticketv1.GetTicketRequest{Id: tk.GetId()})
	if err != nil {
		t.Fatalf("a failed unread clear must not fail the read: %v", err)
	}
	if got.GetId() != tk.GetId() {
		t.Fatalf("id = %q, want %q", got.GetId(), tk.GetId())
	}
	if !got.GetUnread() {
		t.Fatal("the response claims the ticket is read although the clear failed")
	}
	if !repo.tickets[tk.GetId()].AuthorUnread {
		t.Fatal("the stored unread mark was cleared although MarkRead failed")
	}

	// The admin read behaves the same on its own flag.
	if _, err := s.PostTicketMessage(as("p1"), &ticketv1.PostTicketMessageRequest{Id: tk.GetId(), Text: "Спасибо"}); err != nil {
		t.Fatal(err)
	}
	staffGot, err := s.AdminGetTicket(as("admin"), &ticketv1.AdminGetTicketRequest{Id: tk.GetId()})
	if err != nil {
		t.Fatalf("a failed staff unread clear must not fail the admin read: %v", err)
	}
	if !staffGot.GetUnread() || !repo.tickets[tk.GetId()].StaffUnread {
		t.Fatal("the staff unread mark was cleared although MarkRead failed")
	}
}

func TestCloseStale(t *testing.T) {
	s, repo, _ := newTestService()
	tk := create(t, s, "p1")
	if _, err := s.AdminReplyTicket(as("admin"), &ticketv1.AdminReplyTicketRequest{Id: tk.GetId(), Text: "ok"}); err != nil {
		t.Fatal(err)
	}

	// The fake lists every ANSWERED ticket whatever the cutoff, so both
	// halves run through model.Ticket.AutoClose: not due yet, then exactly
	// due.
	s.CloseStale(context.Background(), s.now().Add(model.AutoCloseAfter-time.Hour))
	if repo.tickets[tk.GetId()].IsClosed() {
		t.Fatal("closed too early")
	}
	s.CloseStale(context.Background(), s.now().Add(model.AutoCloseAfter))
	if !repo.tickets[tk.GetId()].IsClosed() {
		t.Fatal("a quiet answered ticket closes itself")
	}
}
