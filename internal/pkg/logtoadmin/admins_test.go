package logtoadmin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap/zapcore"
)

func newTestDirectory(t *testing.T, h http.HandlerFunc) (*Directory, *time.Time) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	d := &Directory{
		client:  srv.Client(),
		baseURL: srv.URL,
		log:     logger.NewWithCore(zapcore.NewNopCore()),
		now:     func() time.Time { return now },
	}
	return d, &now
}

func TestUserIDsFindsAdminRoleAndCaches(t *testing.T) {
	var calls atomic.Int32
	d, now := newTestDirectory(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/api/roles":
			_, _ = w.Write([]byte(`[{"id":"r1","name":"player"},{"id":"r2","name":"admin"}]`))
		case "/api/roles/r2/users":
			_, _ = w.Write([]byte(`[{"id":"u1"},{"id":"u2"}]`))
		default:
			http.NotFound(w, r)
		}
	})

	ids, err := d.UserIDs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "u1" || ids[1] != "u2" {
		t.Fatalf("ids = %v", ids)
	}

	if _, err := d.UserIDs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("cached read should not call logto, calls = %d", calls.Load())
	}

	*now = now.Add(cacheTTL + time.Second)
	if _, err := d.UserIDs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 4 {
		t.Fatalf("expired cache should refetch, calls = %d", calls.Load())
	}
}

func TestUserIDsWithoutAdminRoleIsAnError(t *testing.T) {
	d, _ := newTestDirectory(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"r1","name":"player"}]`))
	})

	if _, err := d.UserIDs(context.Background()); !errors.Is(err, ErrRoleNotFound) {
		t.Fatalf("err = %v, want ErrRoleNotFound", err)
	}
}

func TestUserIDsFallsBackToCacheOnFailure(t *testing.T) {
	fail := false
	d, now := newTestDirectory(t, func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if r.URL.Path == "/api/roles" {
			_, _ = w.Write([]byte(`[{"id":"r2","name":"admin"}]`))
			return
		}
		_, _ = w.Write([]byte(`[{"id":"u1"}]`))
	})

	if _, err := d.UserIDs(context.Background()); err != nil {
		t.Fatal(err)
	}
	fail = true
	*now = now.Add(cacheTTL + time.Second)
	ids, err := d.UserIDs(context.Background())
	if err != nil || len(ids) != 1 {
		t.Fatalf("ids = %v, err = %v; want the cached list", ids, err)
	}
}
