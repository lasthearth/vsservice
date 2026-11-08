package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/ticket/internal/model"
	"go.uber.org/zap/zapcore"
)

type sent struct{ title, body string }

// fakeSite records who got which notification. The options are opaque
// closures, so the recipient is not visible; the count is.
type fakeSite struct{ got []sent }

func (f *fakeSite) CreateNotification(_ context.Context, title, body string, _ ...notificationuc.NotificationOpts) error {
	f.got = append(f.got, sent{title, body})
	return nil
}

type fakeAdmins []string

func (a fakeAdmins) UserIDs(context.Context) ([]string, error) { return a, nil }

func newTestNotifier(site *fakeSite, admins fakeAdmins, webhook string, client *http.Client) *Notifier {
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
