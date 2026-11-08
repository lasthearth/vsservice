package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	notificationv1 "github.com/lasthearth/vsservice/gen/notification/v1"
	"github.com/lasthearth/vsservice/internal/notification/internal/model"
	"github.com/lasthearth/vsservice/internal/notification/internal/service"
	"github.com/lasthearth/vsservice/internal/notification/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// notificationID is a well-formed ObjectID hex, the only id shape the service
// accepts.
const notificationID = "672f2f2f2f2f2f2f2f2f2f2f"

// createdAt backs the canned notification, so the mapper converts a real
// timestamp and not the zero time.
var createdAt = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

// readCall is one MarkNotificationRead the fake saw. The pair is the fix: the
// update is filtered on the caller, so the identity it carried is what every
// test here asserts.
type readCall struct {
	id     string
	userId string
}

// listCall is one ListNotifications the fake saw.
type listCall struct {
	limit     int
	userId    string
	pageToken string
	orderBy   string
}

// fakeRepo implements service.Repository in memory and records every call.
type fakeRepo struct {
	// markErr is what MarkNotificationRead answers, nil for a matched document.
	markErr error
	reads   []readCall
	lists   []listCall
}

func (r *fakeRepo) MarkNotificationRead(_ context.Context, id, userId string) error {
	r.reads = append(r.reads, readCall{id: id, userId: userId})

	return r.markErr
}

func (r *fakeRepo) ListNotifications(
	_ context.Context, limit int, userId, pageToken, orderBy string,
) ([]model.Notification, string, error) {
	r.lists = append(r.lists, listCall{
		limit:     limit,
		userId:    userId,
		pageToken: pageToken,
		orderBy:   orderBy,
	})

	return []model.Notification{{
		Id:        notificationID,
		UserId:    userId,
		Title:     "Season starts",
		Message:   "The server is up.",
		State:     model.NotificationStateUnread,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}}, "next-page", nil
}

// newService builds the service over a zap observer, so a test sees the records
// the call emits. The create use case stays nil: neither method under test
// touches it, and a call that starts touching it panics instead of passing.
func newService(t *testing.T, repo service.Repository) (*service.Service, *observer.ObservedLogs) {
	t.Helper()

	core, logs := observer.New(zapcore.DebugLevel)

	svc := service.New(service.Opts{
		Log:    logger.NewWithCore(core),
		Mapper: &sermapper.NotificationMapperImpl{},
		Repo:   repo,
	})

	return svc, logs
}

// assertOneRecord fails unless exactly one record carries msg at level, and
// returns it.
func assertOneRecord(t *testing.T, logs *observer.ObservedLogs, msg string, level zapcore.Level) observer.LoggedEntry {
	t.Helper()

	entries := logs.FilterMessage(msg).All()
	if len(entries) != 1 {
		t.Fatalf("records named %q = %d, want 1; all records: %v", msg, len(entries), logs.All())
	}
	if entries[0].Level != level {
		t.Fatalf("record %q level = %s, want %s", msg, entries[0].Level, level)
	}

	return entries[0]
}

func assertStringField(t *testing.T, e observer.LoggedEntry, key, want string) {
	t.Helper()

	for _, f := range e.Context {
		if f.Key != key {
			continue
		}
		if f.Type != zapcore.StringType || f.String != want {
			t.Errorf("field %q: want %q, got %+v", key, want, f)
		}

		return
	}
	t.Errorf("record %q has no field %q: %+v", e.Message, key, e.Context)
}

// assertNoErrors fails on any Error record: the happy paths here log nothing.
func assertNoErrors(t *testing.T, logs *observer.ObservedLogs) {
	t.Helper()

	if entries := logs.FilterLevelExact(zapcore.ErrorLevel).All(); len(entries) != 0 {
		t.Errorf("error records = %d, want 0: %v", len(entries), entries)
	}
}

// MarkAsRead must hand the repository the caller's own id. Before the fix it
// passed nothing, and the update matched on _id alone: any caller could mark
// anybody's notification read.
func TestMarkAsReadPassesTheCallerIdentity(t *testing.T) {
	cases := []struct {
		name   string
		caller string
	}{
		{"a player marks their own notification", "user-1"},
		{"a second player gets their own id, not a constant", "user-2"},
		{"a Logto-shaped id is passed through unchanged", "68a1f0c2e4b0a1b2c3d4e5f6"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			svc, logs := newService(t, repo)

			ctx := interceptor.ContextWithUserID(context.Background(), tc.caller)

			resp, err := svc.MarkAsRead(ctx, &notificationv1.MarkAsReadRequest{Id: notificationID})
			if err != nil {
				t.Fatalf("MarkAsRead: %v", err)
			}
			if resp == nil {
				t.Fatal("response = nil, want an empty message")
			}
			if len(repo.reads) != 1 {
				t.Fatalf("repository calls = %v, want exactly one", repo.reads)
			}

			got := repo.reads[0]
			if got.userId != tc.caller {
				t.Errorf("userId = %q, want the caller %q", got.userId, tc.caller)
			}
			if got.userId == "" {
				t.Error("userId is empty: the update would match on _id alone")
			}
			if got.userId == model.BroadcastUserId {
				t.Errorf("userId = %q: the caller must never be replaced by the broadcast owner", got.userId)
			}
			if got.id != notificationID {
				t.Errorf("id = %q, want %q", got.id, notificationID)
			}

			assertNoErrors(t, logs)
		})
	}
}

// Without a caller there is no identity to filter on, so the call fails closed
// and never reaches the database.
func TestMarkAsReadWithoutAUserFails(t *testing.T) {
	repo := &fakeRepo{}
	svc, logs := newService(t, repo)

	resp, err := svc.MarkAsRead(context.Background(), &notificationv1.MarkAsReadRequest{Id: notificationID})

	if code := status.Code(err); code != codes.Internal {
		t.Fatalf("code = %v, want Internal: %v", code, err)
	}
	if resp != nil {
		t.Errorf("response = %v, want nil next to the error", resp)
	}
	if len(repo.reads) != 0 {
		t.Errorf("repository calls = %v, want none", repo.reads)
	}

	e := assertOneRecord(t, logs, "failed to get user id", zapcore.ErrorLevel)
	assertStringField(t, e, "method", "mark_as_read")
}

// The id is parsed first, so a malformed one is an InvalidArgument whatever the
// context carries, and the repository stays untouched.
func TestMarkAsReadRejectsABadId(t *testing.T) {
	cases := []struct {
		name     string
		id       string
		withUser bool
	}{
		{"an empty id", "", true},
		{"plain text", "not-an-object-id", true},
		{"23 hex characters", strings.Repeat("a", 23), true},
		{"25 hex characters", strings.Repeat("a", 25), true},
		{"24 characters that are not hex", strings.Repeat("z", 24), true},
		{"a bad id is rejected before the caller is looked up", "not-an-object-id", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			svc, logs := newService(t, repo)

			ctx := context.Background()
			if tc.withUser {
				ctx = interceptor.ContextWithUserID(ctx, "user-1")
			}

			resp, err := svc.MarkAsRead(ctx, &notificationv1.MarkAsReadRequest{Id: tc.id})

			if code := status.Code(err); code != codes.InvalidArgument {
				t.Fatalf("code = %v, want InvalidArgument: %v", code, err)
			}
			if resp != nil {
				t.Errorf("response = %v, want nil next to the error", resp)
			}
			if len(repo.reads) != 0 {
				t.Errorf("repository calls = %v, want none", repo.reads)
			}

			e := assertOneRecord(t, logs, "failed to parse notification id", zapcore.ErrorLevel)
			assertStringField(t, e, "method", "mark_as_read")
		})
	}
}

// A repository failure is an Internal, and the caller identity still reached the
// repository: the failure is the write, not the lookup.
func TestMarkAsReadReportsARepositoryFailure(t *testing.T) {
	repo := &fakeRepo{markErr: context.DeadlineExceeded}
	svc, _ := newService(t, repo)

	ctx := interceptor.ContextWithUserID(context.Background(), "user-1")

	resp, err := svc.MarkAsRead(ctx, &notificationv1.MarkAsReadRequest{Id: notificationID})

	if code := status.Code(err); code != codes.Internal {
		t.Fatalf("code = %v, want Internal: %v", code, err)
	}
	if resp != nil {
		t.Errorf("response = %v, want nil next to the error", resp)
	}
	if len(repo.reads) != 1 || repo.reads[0].userId != "user-1" {
		t.Errorf("repository calls = %v, want one call carrying user-1", repo.reads)
	}
}

// The list is filtered on the same caller identity, and the mapper is wired: the
// answer carries the notification the repository returned.
func TestListNotificationsFiltersOnTheCaller(t *testing.T) {
	repo := &fakeRepo{}
	svc, logs := newService(t, repo)

	ctx := interceptor.ContextWithUserID(context.Background(), "user-9")

	resp, err := svc.ListNotifications(ctx, &notificationv1.ListNotificationsRequest{
		PageSize: 40,
		OrderBy:  "CREATED_AT DESC",
	})
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if len(repo.lists) != 1 {
		t.Fatalf("repository calls = %v, want exactly one", repo.lists)
	}

	got := repo.lists[0]
	if got.userId != "user-9" {
		t.Errorf("userId = %q, want the caller user-9", got.userId)
	}
	if got.limit != 15 {
		t.Errorf("limit = %d, want the 15 cap", got.limit)
	}
	if got.orderBy != "created_at desc" {
		t.Errorf("orderBy = %q, want it lower-cased", got.orderBy)
	}
	if got.pageToken != "" {
		t.Errorf("pageToken = %q, want it empty", got.pageToken)
	}

	if token := resp.GetNextPageToken(); token != "next-page" {
		t.Errorf("next page token = %q, want next-page", token)
	}
	notifications := resp.GetNotifications()
	if len(notifications) != 1 {
		t.Fatalf("notifications = %v, want the one the repository returned", notifications)
	}
	n := notifications[0]
	if n.GetId() != notificationID {
		t.Errorf("id = %q, want %q", n.GetId(), notificationID)
	}
	if n.GetUserId() != "user-9" {
		t.Errorf("user id = %q, want user-9", n.GetUserId())
	}
	if n.GetState() != notificationv1.Notification_UNREAD {
		t.Errorf("state = %v, want UNREAD", n.GetState())
	}
	if n.GetTitle() != "Season starts" || n.GetMessage() != "The server is up." {
		t.Errorf("title = %q, message = %q; want the canned text", n.GetTitle(), n.GetMessage())
	}
	if ts := n.GetCreatedAt().AsTime(); !ts.Equal(createdAt) {
		t.Errorf("created_at = %s, want %s", ts, createdAt)
	}

	assertNoErrors(t, logs)
}
