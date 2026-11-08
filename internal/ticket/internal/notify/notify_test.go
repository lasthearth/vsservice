package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/ticket/internal/model"
	"github.com/lasthearth/vsservice/internal/ticket/internal/repository/repomapper"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// sent is one site notification the fake received, with the recipient and
// link the options carried.
type sent struct {
	title, body  string
	userId, link string
}

// fakeSite records who got which notification. The options are applied to a
// zero model.Notification exactly the way the real use case applies them. The
// model lives in an internal package this test may not import, so the value is
// built through reflection: an option's own signature names its type.
type fakeSite struct{ got []sent }

func (f *fakeSite) CreateNotification(_ context.Context, title, body string, opts ...notificationuc.NotificationOpts) error {
	var userId, link string
	if len(opts) > 0 {
		n := reflect.New(reflect.TypeOf(opts[0]).In(0).Elem())
		n.Elem().FieldByName("Title").SetString(title)
		n.Elem().FieldByName("Message").SetString(body)
		for _, opt := range opts {
			reflect.ValueOf(opt).Call([]reflect.Value{n})
		}
		userId = n.Elem().FieldByName("UserId").String()
		link = n.Elem().FieldByName("Link").String()
	}
	f.got = append(f.got, sent{title: title, body: body, userId: userId, link: link})
	return nil
}

type fakeAdmins []string

func (a fakeAdmins) UserIDs(context.Context) ([]string, error) { return a, nil }

// failingAdmins stands for a Logto outage.
type failingAdmins struct{}

func (failingAdmins) UserIDs(context.Context) ([]string, error) {
	return nil, errors.New("logto is down")
}

func newTestNotifier(site *fakeSite, admins AdminDirectory, webhook string, client *http.Client) *Notifier {
	return &Notifier{
		log:     logger.NewWithCore(zapcore.NewNopCore()),
		site:    site,
		admins:  admins,
		http:    client,
		webhook: webhook,
		roleID:  "42",
		async:   func(f func()) { f() },
	}
}

func ticket(t *testing.T) *model.Ticket {
	t.Helper()
	tk, err := model.NewTicket("author", "Пропал сундук @everyone", model.CategoryBug, "Помогите @everyone", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return tk
}

// stored round-trips a ticket through its document to give it a real id, the
// only way to get one outside the model package.
var mapper repomapper.MapperImpl

func stored(t *testing.T, tk *model.Ticket) *model.Ticket {
	t.Helper()
	doc := mapper.FromModel(*tk)
	doc.Model = mongox.NewModel()
	doc.CreatedAt = tk.CreatedAt
	out := mapper.ToModel(doc)
	return &out
}

func TestOpenedAlertsEveryAdminButTheAuthor(t *testing.T) {
	site := &fakeSite{}
	n := newTestNotifier(site, fakeAdmins{"a1", "a2", "author"}, "", nil)

	n.TicketOpened(context.Background(), ticket(t))
	if len(site.got) != 2 {
		t.Fatalf("notifications = %d, want 2", len(site.got))
	}
}

func TestAuthorReplyGoesToAssigneeOnly(t *testing.T) {
	site := &fakeSite{}
	n := newTestNotifier(site, fakeAdmins{"a1", "a2"}, "", nil)
	tk := ticket(t)
	tk.Assign("a1")

	n.AuthorWrote(context.Background(), tk, "Медный сундук")
	if len(site.got) != 1 || !strings.Contains(site.got[0].body, "Медный") {
		t.Fatalf("got %+v", site.got)
	}
}

func TestDiscordPayloadNeverMassPings(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	n := newTestNotifier(&fakeSite{}, fakeAdmins{"a1"}, srv.URL, srv.Client())
	n.TicketOpened(context.Background(), ticket(t))

	var msg discordMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatalf("payload: %v (%s)", err, body)
	}
	if len(msg.AllowedMentions.Parse) != 0 {
		t.Fatal("parse must stay empty so @everyone in player text pings nobody")
	}
	if msg.Content != "<@&42>" || len(msg.AllowedMentions.Roles) != 1 {
		t.Fatalf("a new ticket pings the role: %+v", msg)
	}
	if !strings.HasPrefix(msg.Embeds[0].URL, SiteURL+"/profile/admin?tab=tickets&ticket=") {
		t.Fatalf("url = %s", msg.Embeds[0].URL)
	}
}

func TestExcerpt(t *testing.T) {
	if got := excerpt("абвгд", 3); got != "аб…" {
		t.Fatalf("got %q", got)
	}
	if got := excerpt(" ok ", 3); got != "ok" {
		t.Fatalf("got %q", got)
	}
}

func TestStaffRepliedGoesToTheAuthorOnly(t *testing.T) {
	site := &fakeSite{}
	n := newTestNotifier(site, fakeAdmins{"a1", "a2"}, "", nil)
	tk := stored(t, ticket(t))

	n.StaffReplied(context.Background(), tk)

	if len(site.got) != 1 {
		t.Fatalf("notifications = %d, want exactly 1: %+v", len(site.got), site.got)
	}
	if site.got[0].userId != "author" {
		t.Fatalf("recipient = %q, want the author", site.got[0].userId)
	}
	if site.got[0].link != AuthorLink(tk.Id) {
		t.Fatalf("link = %q, want AuthorLink %q", site.got[0].link, AuthorLink(tk.Id))
	}
}

func TestToStaffLinksToTheAdminInbox(t *testing.T) {
	site := &fakeSite{}
	n := newTestNotifier(site, fakeAdmins{"a1", "a2"}, "", nil)
	tk := stored(t, ticket(t))
	tk.Assign("a1")

	n.AuthorWrote(context.Background(), tk, "Медный сундук")

	if len(site.got) != 1 {
		t.Fatalf("notifications = %d, want exactly 1: %+v", len(site.got), site.got)
	}
	if site.got[0].userId != "a1" {
		t.Fatalf("recipient = %q, want the assignee", site.got[0].userId)
	}
	if site.got[0].link != StaffLink(tk.Id) {
		t.Fatalf("link = %q, want StaffLink %q", site.got[0].link, StaffLink(tk.Id))
	}
}

func TestToStaffSkipsWhenAdminsFail(t *testing.T) {
	site := &fakeSite{}
	n := newTestNotifier(site, failingAdmins{}, "", nil)

	// An unassigned ticket needs the admin directory; a failure must skip
	// the alerts, not panic and not fall back to somebody.
	n.TicketOpened(context.Background(), stored(t, ticket(t)))

	if len(site.got) != 0 {
		t.Fatalf("notifications = %d, want 0: %+v", len(site.got), site.got)
	}
}

func TestRunRecoversFromAPanic(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	n := &Notifier{
		log:    logger.NewWithCore(core),
		site:   &fakeSite{},
		admins: fakeAdmins{"a1"},
		async:  func(f func()) { f() },
	}

	// A panic escaping the delivery closure would take the process down on
	// the real goroutine; run must contain it and say so in the log.
	n.run(context.Background(), "panic_test", func(context.Context) { panic("boom") })

	if logs.FilterMessage("ticket alert panicked").Len() != 1 {
		t.Fatalf("the recovered panic was not logged once: %+v", logs.All())
	}
}

func TestWebhookDisabledSendsNothing(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	tk := stored(t, ticket(t))

	// A disabled webhook must not reach the client at all: without the early
	// return, building the request for an empty URL fails and logs.
	core, logs := observer.New(zapcore.DebugLevel)
	n := &Notifier{
		log:     logger.NewWithCore(core),
		site:    &fakeSite{},
		admins:  fakeAdmins{"a1"},
		http:    srv.Client(),
		webhook: "",
		roleID:  "42",
		async:   func(f func()) { f() },
	}
	n.TicketOpened(context.Background(), tk)
	if hits != 0 {
		t.Fatalf("an empty webhook performed %d requests", hits)
	}
	if logs.Len() != 0 {
		t.Fatalf("a disabled webhook must stay silent, got %d entries: %+v", logs.Len(), logs.All())
	}

	// The same wiring with a configured webhook does reach the server, so
	// the zero above is the guard, not a dead counter.
	n = newTestNotifier(&fakeSite{}, fakeAdmins{"a1"}, srv.URL, srv.Client())
	n.TicketOpened(context.Background(), tk)
	if hits != 1 {
		t.Fatalf("a configured webhook performed %d requests, want 1", hits)
	}
}
