// Package notify tells the other side of a ticket that something happened:
// the staff on the site and in their Discord channel, the author on the site.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/httpclient"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/logtoadmin"
	"github.com/lasthearth/vsservice/internal/ticket/internal/model"
	"github.com/lasthearth/vsservice/internal/ticket/internal/service"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// SiteURL is where the links in Discord point.
const SiteURL = "https://lasthearth.ru"

// deliverTimeout bounds one delivery. It runs after the response is sent, so
// the caller never waits for Logto or Discord.
const deliverTimeout = 30 * time.Second

var _ service.Notifier = (*Notifier)(nil)

// CategoryLabels are the Russian category names in notification texts.
var CategoryLabels = map[model.Category]string{
	model.CategoryBug:        "Баг",
	model.CategoryComplaint:  "Жалоба на игрока",
	model.CategoryDonate:     "Донат и покупки",
	model.CategorySettlement: "Поселение",
	model.CategoryQuestion:   "Вопрос",
	model.CategoryOther:      "Другое",
}

// SiteNotifier sends one site notification; *notificationuc.Create in
// production.
type SiteNotifier interface {
	CreateNotification(ctx context.Context, title, message string, opts ...notificationuc.NotificationOpts) error
}

// AdminDirectory lists the administrators; *logtoadmin.Directory in production.
type AdminDirectory interface {
	UserIDs(ctx context.Context) ([]string, error)
}

type Opts struct {
	fx.In
	Lc     fx.Lifecycle
	Logger logger.Logger
	Cfg    config.Config
	Site   *notificationuc.Create
	Admins *logtoadmin.Directory
	HTTP   *http.Client
}

// Notifier delivers ticket alerts in the background.
type Notifier struct {
	log     logger.Logger
	site    SiteNotifier
	admins  AdminDirectory
	http    *http.Client
	webhook string
	roleID  string
	// async runs a delivery; a goroutine in production, inline in tests.
	async func(func())
	// stateMu guards draining and keeps inflight.Add away from inflight.Wait,
	// which the WaitGroup does not allow to overlap.
	stateMu  sync.Mutex
	draining bool
	// inflight counts the deliveries async started, so shutdown waits for
	// them instead of cutting them off mid-write.
	inflight sync.WaitGroup
}

func New(opts Opts) *Notifier {
	n := &Notifier{
		log:     opts.Logger.WithComponent("notify"),
		site:    opts.Site,
		admins:  opts.Admins,
		http:    opts.HTTP,
		webhook: strings.TrimSpace(opts.Cfg.DiscordTicketsWebhook),
		roleID:  strings.TrimSpace(opts.Cfg.DiscordTicketsRoleID),
		async:   func(f func()) { go f() },
	}
	opts.Lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			n.stateMu.Lock()
			n.draining = true
			n.stateMu.Unlock()

			waited := make(chan struct{})
			go func() {
				defer close(waited)
				n.inflight.Wait()
			}()
			select {
			case <-waited:
			case <-ctx.Done():
				n.log.Warn("ticket alerts still in flight at shutdown")
			}
			return nil
		},
	})
	return n
}

// start reports whether a delivery may still begin, and counts it when it may.
// After shutdown starts, alerts are dropped: the process is going away and the
// database it writes to is closing too.
func (n *Notifier) start() bool {
	n.stateMu.Lock()
	defer n.stateMu.Unlock()
	if n.draining {
		return false
	}
	n.inflight.Add(1)
	return true
}

// StaffLink is the ticket in the admin inbox.
func StaffLink(id string) string { return "/profile/admin?tab=tickets&ticket=" + id }

// AuthorLink is the ticket on the player's support page.
func AuthorLink(id string) string { return "/profile/support/" + id }

// TicketOpened implements service.Notifier.
func (n *Notifier) TicketOpened(ctx context.Context, t *model.Ticket) {
	snap := *t
	first := ""
	if len(t.Messages) > 0 {
		first = t.Messages[0].Text
	}
	n.run(ctx, "ticket_opened", func(ctx context.Context) {
		n.toStaff(ctx, &snap, "Новое обращение",
			fmt.Sprintf("«%s» — %s", snap.Subject, CategoryLabels[snap.Category]))
		n.toDiscord(ctx, &snap, "Новое обращение", first, true)
	})
}

// AuthorWrote implements service.Notifier. An empty text means the author
// reopened the ticket.
func (n *Notifier) AuthorWrote(ctx context.Context, t *model.Ticket, text string) {
	snap := *t
	title, body := "Ответ в обращении", fmt.Sprintf("«%s»: %s", snap.Subject, excerpt(text, 120))
	if strings.TrimSpace(text) == "" {
		title, body = "Обращение открыто снова", fmt.Sprintf("«%s»", snap.Subject)
	}
	n.run(ctx, "ticket_author_wrote", func(ctx context.Context) {
		n.toStaff(ctx, &snap, title, body)
		n.toDiscord(ctx, &snap, title, text, false)
	})
}

// StaffReplied implements service.Notifier.
func (n *Notifier) StaffReplied(ctx context.Context, t *model.Ticket) {
	snap := *t
	body := fmt.Sprintf("Администрация ответила в обращении «%s»", snap.Subject)
	if snap.IsClosed() {
		body = fmt.Sprintf("Обращение «%s» закрыто с ответом администрации", snap.Subject)
	}
	n.run(ctx, "ticket_staff_replied", func(ctx context.Context) {
		err := n.site.CreateNotification(ctx, "Ответ на обращение", body,
			notificationuc.WithUserId(snap.AuthorId), notificationuc.WithLink(AuthorLink(snap.Id)))
		if err != nil {
			logger.From(ctx, n.log).Warn("failed to notify ticket author",
				zap.String("ticket_id", snap.Id), zap.Error(err))
		}
	})
}

// run detaches a delivery from the request: the response does not wait for
// it, and the request's cancellation does not cut it short.
func (n *Notifier) run(ctx context.Context, job string, f func(ctx context.Context)) {
	if !n.start() {
		return
	}
	base := context.WithoutCancel(ctx)
	n.async(func() {
		defer n.inflight.Done()
		ctx, _ := logger.WithJob(base, job)
		ctx, cancel := context.WithTimeout(ctx, deliverTimeout)
		defer cancel()
		// A delivery runs on its own goroutine, where a panic would take the
		// whole service down. Nothing a delivery touches is worth that.
		defer func() {
			if r := recover(); r != nil {
				logger.From(ctx, n.log).Error("ticket alert panicked",
					zap.String("job", job), zap.Any("panic", r))
			}
		}()
		f(ctx)
	})
}

// toStaff notifies the assignee or, while nobody took the ticket, every admin.
func (n *Notifier) toStaff(ctx context.Context, t *model.Ticket, title, body string) {
	log := logger.From(ctx, n.log).With(zap.String("ticket_id", t.Id))

	recipients := []string{t.AssigneeId}
	if t.AssigneeId == "" {
		ids, err := n.admins.UserIDs(ctx)
		if err != nil {
			log.Warn("failed to list admins for a ticket alert", zap.Error(err))
			return
		}
		recipients = ids
	}

	for _, id := range recipients {
		// An admin who opened a ticket of their own needs no alert about it.
		if id == "" || id == t.AuthorId {
			continue
		}
		err := n.site.CreateNotification(ctx, title, body,
			notificationuc.WithUserId(id), notificationuc.WithLink(StaffLink(t.Id)))
		if err != nil {
			log.Warn("failed to notify admin", zap.String("admin_id", id), zap.Error(err))
		}
	}
}

// toDiscord posts to the staff channel when one is configured.
func (n *Notifier) toDiscord(ctx context.Context, t *model.Ticket, title, text string, ping bool) {
	if n.webhook == "" {
		return
	}
	payload, err := json.Marshal(discordPayload(t, title, text, n.roleID, ping))
	if err != nil {
		logger.From(ctx, n.log).Warn("failed to encode discord payload",
			zap.String("ticket_id", t.Id), zap.Error(err))
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhook, bytes.NewReader(payload))
	if err != nil {
		logger.From(ctx, n.log).Warn("failed to build discord request",
			httpclient.MaskedError(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LastHearthBot (https://lasthearth.ru, 1.0)")

	resp, err := n.http.Do(req)
	if err != nil {
		// The error text carries the webhook URL, and the token is part of it.
		logger.From(ctx, n.log).Warn("discord ticket alert failed",
			zap.String("ticket_id", t.Id), httpclient.MaskedError(err))
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		logger.From(ctx, n.log).Warn("discord ticket alert rejected",
			zap.String("ticket_id", t.Id), zap.Int("status", resp.StatusCode), zap.ByteString("body", body))
		return
	}
	// Drain so the client can reuse the connection.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 2048))
}

type discordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type discordEmbed struct {
	Title       string              `json:"title"`
	URL         string              `json:"url"`
	Description string              `json:"description,omitempty"`
	Color       int                 `json:"color"`
	Fields      []discordEmbedField `json:"fields"`
	Timestamp   string              `json:"timestamp"`
}

type discordAllowedMentions struct {
	// Parse is always empty: a player's text must not ping @everyone or a
	// role by writing its name.
	Parse []string `json:"parse"`
	Roles []string `json:"roles"`
}

type discordMessage struct {
	Content         string                 `json:"content,omitempty"`
	Embeds          []discordEmbed         `json:"embeds"`
	AllowedMentions discordAllowedMentions `json:"allowed_mentions"`
}

// discordPayload is the webhook body: an embed linking to the admin inbox,
// plus a role ping for a new ticket when a role is configured.
func discordPayload(t *model.Ticket, title, text, roleID string, ping bool) discordMessage {
	msg := discordMessage{
		Embeds: []discordEmbed{{
			Title:       excerpt(title+": "+t.Subject, 250),
			URL:         SiteURL + StaffLink(t.Id),
			Description: excerpt(text, 500),
			Color:       0xC8963E,
			Fields: []discordEmbedField{
				{Name: "Категория", Value: CategoryLabels[t.Category], Inline: true},
				{Name: "Сообщений", Value: strconv.Itoa(int(t.PublicCount)), Inline: true},
			},
			Timestamp: t.LastMessageAt.UTC().Format(time.RFC3339),
		}},
		AllowedMentions: discordAllowedMentions{Parse: []string{}, Roles: []string{}},
	}
	if ping && roleID != "" {
		msg.Content = "<@&" + roleID + ">"
		msg.AllowedMentions.Roles = []string{roleID}
	}
	return msg
}

// excerpt cuts s to limit characters, marking the cut with an ellipsis.
func excerpt(s string, limit int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	r := []rune(s)
	return string(r[:limit-1]) + "…"
}
