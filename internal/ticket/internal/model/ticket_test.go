package model

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func newOpen(t *testing.T) *Ticket {
	t.Helper()
	tk, err := NewTicket("player", "Пропал сундук", CategoryBug, "Вчера был, сегодня нет", t0)
	if err != nil {
		t.Fatal(err)
	}
	return tk
}

func TestNewTicketValidates(t *testing.T) {
	cases := []struct {
		name     string
		author   string
		subject  string
		category Category
		text     string
		want     error
	}{
		{"no author", " ", "Тема", CategoryBug, "текст", ErrAuthorRequired},
		{"unknown category", "p", "Тема", "weird", "текст", ErrCategoryInvalid},
		{"short subject", "p", "ab", CategoryBug, "текст", ErrSubjectInvalid},
		{"long subject", "p", strings.Repeat("я", SubjectMaxLen+1), CategoryBug, "текст", ErrSubjectInvalid},
		{"blank text", "p", "Тема", CategoryBug, "   ", ErrTextInvalid},
		{"long text", "p", "Тема", CategoryBug, strings.Repeat("я", TextMaxLen+1), ErrTextInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewTicket(c.author, c.subject, c.category, c.text, t0); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestNewTicketIsOpenAndUnreadForStaff(t *testing.T) {
	tk := newOpen(t)
	if tk.Status != StatusOpen || !tk.StaffUnread || tk.AuthorUnread {
		t.Fatalf("status %s staffUnread %v authorUnread %v", tk.Status, tk.StaffUnread, tk.AuthorUnread)
	}
	if tk.MessageCount != 1 || tk.PublicCount != 1 || tk.Messages[0].Staff {
		t.Fatalf("first message wrong: %+v", tk.Messages)
	}
}

func TestStaffReplyAnswersAndTakesTicket(t *testing.T) {
	tk := newOpen(t)
	at := t0.Add(time.Hour)
	if err := tk.ReplyByStaff("admin", "Проверим", false, false, at); err != nil {
		t.Fatal(err)
	}
	if tk.Status != StatusAnswered || tk.AnsweredAt == nil || !tk.AnsweredAt.Equal(at) {
		t.Fatalf("status %s answeredAt %v", tk.Status, tk.AnsweredAt)
	}
	if tk.AssigneeId != "admin" || !tk.AuthorUnread || tk.StaffUnread {
		t.Fatalf("assignee %q authorUnread %v staffUnread %v", tk.AssigneeId, tk.AuthorUnread, tk.StaffUnread)
	}
	if !tk.LastPublicAt.Equal(at) {
		t.Fatal("a reply is public activity")
	}
}

func TestInternalNoteChangesNothingForAuthor(t *testing.T) {
	tk := newOpen(t)
	at := t0.Add(time.Hour)
	if err := tk.ReplyByStaff("admin", "Похоже на дюп", true, false, at); err != nil {
		t.Fatal(err)
	}
	if tk.Status != StatusOpen || tk.AuthorUnread {
		t.Fatalf("a note must not answer: status %s authorUnread %v", tk.Status, tk.AuthorUnread)
	}
	if !tk.LastPublicAt.Equal(t0) || !tk.LastMessageAt.Equal(at) {
		t.Fatal("a note moves only the staff's last activity")
	}
	if tk.PublicCount != 1 || tk.MessageCount != 2 || len(tk.PublicMessages()) != 1 {
		t.Fatalf("counts public %d all %d", tk.PublicCount, tk.MessageCount)
	}
	if err := tk.ReplyByStaff("admin", "x", true, true, at); !errors.Is(err, ErrCloseWithNote) {
		t.Fatalf("closing with a note: %v", err)
	}
}

func TestReplyKeepsExistingAssignee(t *testing.T) {
	tk := newOpen(t)
	tk.Assign("first")
	if err := tk.ReplyByStaff("second", "ok", false, false, t0); err != nil {
		t.Fatal(err)
	}
	if tk.AssigneeId != "first" {
		t.Fatalf("assignee = %q", tk.AssigneeId)
	}
}

func TestAuthorMessageReopensTheQueue(t *testing.T) {
	tk := newOpen(t)
	_ = tk.ReplyByStaff("admin", "Что за сундук?", false, false, t0)
	if err := tk.PostByAuthor("player", "Медный", t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if tk.Status != StatusOpen || tk.AnsweredAt != nil || !tk.StaffUnread {
		t.Fatalf("status %s answeredAt %v staffUnread %v", tk.Status, tk.AnsweredAt, tk.StaffUnread)
	}
	if err := tk.PostByAuthor("stranger", "я тоже", t0); !errors.Is(err, ErrNotAuthor) {
		t.Fatalf("stranger: %v", err)
	}
}

func TestClosedTicketTakesNoAuthorMessagesUntilReopened(t *testing.T) {
	tk := newOpen(t)
	if err := tk.CloseByAuthor("player", t0); err != nil {
		t.Fatal(err)
	}
	if err := tk.PostByAuthor("player", "ещё", t0); !errors.Is(err, ErrClosed) {
		t.Fatalf("post to closed: %v", err)
	}
	if err := tk.ReopenByAuthor("player", t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if tk.Status != StatusOpen || tk.ClosedAt != nil {
		t.Fatalf("reopen: status %s closedAt %v", tk.Status, tk.ClosedAt)
	}
	if err := tk.ReopenByAuthor("player", t0); !errors.Is(err, ErrNotClosed) {
		t.Fatalf("reopen open: %v", err)
	}
}

func TestStaffReplyCloseAndReopen(t *testing.T) {
	tk := newOpen(t)
	if err := tk.ReplyByStaff("admin", "Вернули сундук", false, true, t0); err != nil {
		t.Fatal(err)
	}
	if !tk.IsClosed() || tk.ClosedAt == nil || !tk.AuthorUnread {
		t.Fatalf("reply and close: status %s", tk.Status)
	}
	if err := tk.ReplyByStaff("admin", "Дополню", false, false, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if tk.Status != StatusAnswered || tk.ClosedAt != nil {
		t.Fatalf("a reply to a closed ticket reopens it as answered: %s", tk.Status)
	}
}

func TestThreadIsCapped(t *testing.T) {
	tk := newOpen(t)
	for i := 1; i < MaxMessages; i++ {
		if err := tk.ReplyByStaff("admin", "n", true, false, t0); err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
	}
	if err := tk.PostByAuthor("player", "ещё", t0); !errors.Is(err, ErrThreadFull) {
		t.Fatalf("over the cap: %v", err)
	}
}

func TestAutoClose(t *testing.T) {
	tk := newOpen(t)
	if tk.AutoClose(t0.Add(30 * 24 * time.Hour)) {
		t.Fatal("an open ticket never closes itself")
	}
	_ = tk.ReplyByStaff("admin", "ok", false, false, t0)
	if tk.AutoClose(t0.Add(AutoCloseAfter - time.Minute)) {
		t.Fatal("too early")
	}
	if !tk.AutoClose(t0.Add(AutoCloseAfter)) || !tk.IsClosed() {
		t.Fatal("a quiet answered ticket closes itself")
	}
}

func TestSetStatusAndCategory(t *testing.T) {
	tk := newOpen(t)
	if err := tk.SetStatus("weird", t0); !errors.Is(err, ErrStatusInvalid) {
		t.Fatal(err)
	}
	if err := tk.SetCategory("weird"); !errors.Is(err, ErrCategoryInvalid) {
		t.Fatal(err)
	}
	if err := tk.SetStatus(StatusClosed, t0); err != nil || !tk.IsClosed() {
		t.Fatal("close by status")
	}
	if err := tk.SetStatus(StatusOpen, t0); err != nil || tk.ClosedAt != nil {
		t.Fatal("open by status")
	}
}

func TestSavedReply(t *testing.T) {
	if _, err := NewSavedReply(" ", "текст", "a", t0); !errors.Is(err, ErrReplyTitleInvalid) {
		t.Fatal(err)
	}
	r, err := NewSavedReply("Приветствие", "Здравствуйте, {nickname}!", "a", t0)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Edit("Привет", "", "b", t0); !errors.Is(err, ErrTextInvalid) {
		t.Fatal(err)
	}
	if r.Title != "Приветствие" {
		t.Fatal("a failed edit must not change the reply")
	}
}
