package logtoadmin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// testBaseTime is the clock every fixed-clock test starts from, so a case
// compares a number and not a reading of time.Now.
var testBaseTime = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

// newDirectory builds a Directory over an httptest server with the given logger
// and clock.
func newDirectory(t *testing.T, h http.HandlerFunc, log logger.Logger, now func() time.Time) *Directory {
	t.Helper()

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return &Directory{
		client:  srv.Client(),
		baseURL: srv.URL,
		log:     log,
		now:     now,
	}
}

// fixedClock returns a clock pinned to testBaseTime and the pointer that moves
// it: a test ages the cache by writing to the pointer.
func fixedClock() (func() time.Time, *time.Time) {
	now := testBaseTime
	return func() time.Time { return now }, &now
}

func newTestDirectory(t *testing.T, h http.HandlerFunc) (*Directory, *time.Time) {
	t.Helper()

	clock, now := fixedClock()

	return newDirectory(t, h, logger.NewWithCore(zapcore.NewNopCore()), clock), now
}

// newObservedDirectory is newTestDirectory with a zap observer, so a test can
// assert the records a refresh emits instead of only its return value.
func newObservedDirectory(t *testing.T, h http.HandlerFunc) (*Directory, *time.Time, *observer.ObservedLogs) {
	t.Helper()

	clock, now := fixedClock()
	core, logs := observer.New(zapcore.DebugLevel)

	return newDirectory(t, h, logger.NewWithCore(core), clock), now, logs
}

// queryLog records "path?query" for every request a handler saw. The walk is
// sequential, but the handler runs on its own goroutine, so the log keeps a
// mutex.
type queryLog struct {
	mu      sync.Mutex
	entries []string
}

func (q *queryLog) add(r *http.Request) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.entries = append(q.entries, r.URL.Path+"?"+r.URL.RawQuery)
}

func (q *queryLog) all() []string {
	q.mu.Lock()
	defer q.mu.Unlock()

	return slices.Clone(q.entries)
}

// pageItem is one element of a Logto list response.
type pageItem struct {
	Id string `json:"id"`
}

// userPage is a JSON array of n users, ids "<tag>-000" and up.
func userPage(tag string, n int) string {
	b := &strings.Builder{}
	b.WriteString("[")
	for i := range n {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(b, `{"id":%q}`, fmt.Sprintf("%s-%03d", tag, i))
	}
	b.WriteString("]")

	return b.String()
}

// rolePage is a JSON array of n roles, none of them named RoleName.
func rolePage(tag string, n int) string {
	b := &strings.Builder{}
	b.WriteString("[")
	for i := range n {
		if i > 0 {
			b.WriteString(",")
		}
		name := fmt.Sprintf("%s-%03d", tag, i)
		fmt.Fprintf(b, `{"id":%q,"name":%q}`, name, name)
	}
	b.WriteString("]")

	return b.String()
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

func assertIntField(t *testing.T, e observer.LoggedEntry, key string, want int) {
	t.Helper()

	for _, f := range e.Context {
		if f.Key != key {
			continue
		}
		if f.Integer != int64(want) {
			t.Errorf("field %q: want %d, got %d", key, want, f.Integer)
		}

		return
	}
	t.Errorf("record %q has no field %q: %+v", e.Message, key, e.Context)
}

// assertOneRecord fails unless exactly one record carries msg, and returns it.
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

// A failed refresh buys retryBackoff of quiet. Without it a Logto outage makes
// every ticket message repeat a fetch that holds the mutex for its duration.
func TestFailedRefreshServesTheStaleListWithoutCallingLogtoAgain(t *testing.T) {
	const cachedWarn = "failed to refresh admins, using the cached list"

	var calls atomic.Int32
	var fail atomic.Bool
	d, now, logs := newObservedDirectory(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if fail.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if r.URL.Path == "/api/roles" {
			_, _ = w.Write([]byte(`[{"id":"r2","name":"admin"}]`))

			return
		}
		_, _ = w.Write([]byte(`[{"id":"u1"},{"id":"u2"}]`))
	})

	want := []string{"u1", "u2"}

	ids, err := d.UserIDs(context.Background())
	if err != nil {
		t.Fatalf("warm-up UserIDs: %v", err)
	}
	if !slices.Equal(ids, want) {
		t.Fatalf("warm-up ids = %v, want %v", ids, want)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("warm-up calls = %d, want 2 (roles and role users)", got)
	}

	fail.Store(true)
	*now = now.Add(cacheTTL + time.Second)

	ids, err = d.UserIDs(context.Background())
	if err != nil {
		t.Fatalf("UserIDs on a failed refresh: %v", err)
	}
	if !slices.Equal(ids, want) {
		t.Fatalf("ids = %v, want the stale list %v", ids, want)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("calls = %d, want 3: the failed refresh costs one request", got)
	}
	e := assertOneRecord(t, logs, cachedWarn, zapcore.WarnLevel)
	if len(e.Context) == 0 {
		t.Fatalf("record %q carries no error field: %+v", cachedWarn, e.Context)
	}

	// Still inside retryBackoff: three more callers must not reach Logto.
	for i := range 3 {
		ids, err = d.UserIDs(context.Background())
		if err != nil {
			t.Fatalf("caller %d inside the backoff: %v", i, err)
		}
		if !slices.Equal(ids, want) {
			t.Fatalf("caller %d ids = %v, want the stale list %v", i, ids, want)
		}
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("calls = %d, want 3: retryBackoff must suppress the refetch", got)
	}
	if got := logs.FilterMessage(cachedWarn).Len(); got != 1 {
		t.Fatalf("warnings = %d, want 1: a suppressed refetch logs nothing", got)
	}

	*now = now.Add(retryBackoff + time.Second)

	ids, err = d.UserIDs(context.Background())
	if err != nil {
		t.Fatalf("UserIDs past the backoff: %v", err)
	}
	if !slices.Equal(ids, want) {
		t.Fatalf("ids = %v, want the stale list %v", ids, want)
	}
	if got := calls.Load(); got != 4 {
		t.Fatalf("calls = %d, want 4: past retryBackoff the refresh runs again", got)
	}
}

// With no cached list there is nothing to fall back to, so the caller gets the
// error, and the backoff must not swallow the next attempt either.
func TestColdCacheFailureReturnsTheError(t *testing.T) {
	const cachedWarn = "failed to refresh admins, using the cached list"

	var calls atomic.Int32
	d, _, logs := newObservedDirectory(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	})

	for attempt := range 2 {
		ids, err := d.UserIDs(context.Background())
		if err == nil {
			t.Fatalf("attempt %d: err = nil, ids = %v; want the fetch error", attempt, ids)
		}
		if ids != nil {
			t.Fatalf("attempt %d: ids = %v, want nil", attempt, ids)
		}
		if !strings.Contains(err.Error(), "502") {
			t.Fatalf("attempt %d: err = %v, want the logto status in it", attempt, err)
		}
		if got := calls.Load(); got != int32(attempt+1) {
			t.Fatalf("attempt %d: calls = %d, want %d; a cold cache must keep trying",
				attempt, got, attempt+1)
		}
	}

	if got := logs.FilterMessage(cachedWarn).Len(); got != 0 {
		t.Fatalf("warnings = %d, want 0: there is no cached list to fall back to", got)
	}
	if got := logs.FilterLevelExact(zapcore.ErrorLevel).Len(); got != 0 {
		t.Fatalf("error records = %d, want 0: the caller gets the error, not a log", got)
	}
}

// getAll must walk every page. The first page alone hid the admin role as soon
// as Logto held more than pageSize roles.
func TestGetAllWalksEveryPage(t *testing.T) {
	t.Run("collects every page and stops on a short one", func(t *testing.T) {
		q := &queryLog{}
		d, _ := newTestDirectory(t, func(w http.ResponseWriter, r *http.Request) {
			q.add(r)

			page, err := strconv.Atoi(r.URL.Query().Get("page"))
			if err != nil {
				t.Errorf("page is not a number: %v", err)
				http.Error(w, "bad page", http.StatusBadRequest)

				return
			}
			n := pageSize
			if page == 3 {
				n = 7
			}
			if page > 3 {
				t.Errorf("page %d must not be requested: page 3 was short", page)
				n = 0
			}
			_, _ = w.Write([]byte(userPage(fmt.Sprintf("p%d", page), n)))
		})

		var got []pageItem
		if err := getAll(context.Background(), d, "/api/things", &got); err != nil {
			t.Fatalf("getAll: %v", err)
		}

		if want := 2*pageSize + 7; len(got) != want {
			t.Fatalf("collected %d items, want %d", len(got), want)
		}
		if got[0].Id != "p1-000" {
			t.Errorf("first item = %q, want p1-000", got[0].Id)
		}
		if got[pageSize].Id != "p2-000" {
			t.Errorf("item %d = %q, want p2-000: page 2 is missing", pageSize, got[pageSize].Id)
		}
		if last := got[len(got)-1].Id; last != "p3-006" {
			t.Errorf("last item = %q, want p3-006", last)
		}

		wantQueries := []string{
			"/api/things?page_size=100&page=1",
			"/api/things?page_size=100&page=2",
			"/api/things?page_size=100&page=3",
		}
		if queries := q.all(); !slices.Equal(queries, wantQueries) {
			t.Fatalf("queries = %v, want %v", queries, wantQueries)
		}
	})

	t.Run("finds the admin role past the first page", func(t *testing.T) {
		q := &queryLog{}
		d, _ := newTestDirectory(t, func(w http.ResponseWriter, r *http.Request) {
			q.add(r)

			switch r.URL.Path {
			case "/api/roles":
				if r.URL.Query().Get("page") == "1" {
					// A full page of filler roles: "admin" is not on it.
					_, _ = w.Write([]byte(rolePage("filler", pageSize)))

					return
				}
				_, _ = w.Write([]byte(`[{"id":"admin-role","name":"admin"}]`))
			case "/api/roles/admin-role/users":
				_, _ = w.Write([]byte(`[{"id":"a1"},{"id":"a2"}]`))
			default:
				http.NotFound(w, r)
			}
		})

		ids, err := d.UserIDs(context.Background())
		if err != nil {
			t.Fatalf("UserIDs: %v", err)
		}
		if want := []string{"a1", "a2"}; !slices.Equal(ids, want) {
			t.Fatalf("ids = %v, want %v", ids, want)
		}

		queries := q.all()
		if !slices.Contains(queries, "/api/roles?page_size=100&page=2") {
			t.Fatalf("page 2 of the roles was never requested: %v", queries)
		}
		if !slices.Contains(queries, "/api/roles/admin-role/users?page_size=100&page=1") {
			t.Fatalf("the role users were never requested: %v", queries)
		}
	})
}

// maxPages bounds one refresh. Reaching it means the rest of the list is
// invisible, so it must be said out loud.
func TestGetAllStopsAtMaxPages(t *testing.T) {
	const truncatedWarn = "logto list truncated"

	var calls atomic.Int32
	q := &queryLog{}
	d, _, logs := newObservedDirectory(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		q.add(r)
		_, _ = w.Write([]byte(userPage("full", pageSize)))
	})

	var got []pageItem
	if err := getAll(context.Background(), d, "/api/things", &got); err != nil {
		t.Fatalf("getAll: %v", err)
	}

	if n := calls.Load(); n != maxPages {
		t.Fatalf("requests = %d, want maxPages = %d", n, maxPages)
	}
	if want := maxPages * pageSize; len(got) != want {
		t.Fatalf("collected %d items, want %d", len(got), want)
	}
	queries := q.all()
	// The literals pin the reviewed bound: 20 pages of 100, no more.
	if len(queries) != 20 {
		t.Fatalf("requests = %d, want 20: %v", len(queries), queries)
	}
	if first, last := queries[0], queries[len(queries)-1]; first != "/api/things?page_size=100&page=1" ||
		last != "/api/things?page_size=100&page=20" {
		t.Fatalf("first request = %q, last = %q; want page 1 and page 20 at page_size=100", first, last)
	}

	e := assertOneRecord(t, logs, truncatedWarn, zapcore.WarnLevel)
	assertStringField(t, e, "path", "/api/things")
	assertIntField(t, e, "pages", maxPages)
}

// Run with -race. Sixteen readers and a clock that expires the cache four times
// while they run: every caller gets the whole list, and every caller owns its
// own slice.
func TestUserIDsIsSafeUnderConcurrentUse(t *testing.T) {
	want := []string{"u1", "u2", "u3", "u4"}

	var calls atomic.Int32
	var tick atomic.Int64

	d := newDirectory(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/api/roles":
			_, _ = w.Write([]byte(`[{"id":"r2","name":"admin"}]`))
		case "/api/roles/r2/users":
			_, _ = w.Write([]byte(`[{"id":"u1"},{"id":"u2"},{"id":"u3"},{"id":"u4"}]`))
		default:
			http.NotFound(w, r)
		}
	}, logger.NewWithCore(zapcore.NewNopCore()),
		// The clock reads an atomic: the advancer goroutine moves it while the
		// readers are inside UserIDs.
		func() time.Time { return testBaseTime.Add(time.Duration(tick.Load()) * time.Second) })

	const readers = 16
	results := make([][]string, readers)

	clockDone := make(chan struct{})
	go func() {
		defer close(clockDone)
		// 1200 fake seconds at 250 us a step: cacheTTL passes four times in
		// about 300 ms of real time.
		for i := int64(1); i <= 1200; i++ {
			tick.Store(i)
			time.Sleep(250 * time.Microsecond)
		}
	}()

	var wg sync.WaitGroup
	wg.Add(readers)
	for i := range readers {
		go func(i int) {
			defer wg.Done()

			for {
				ids, err := d.UserIDs(context.Background())
				if err != nil {
					t.Errorf("reader %d: UserIDs: %v", i, err)

					return
				}
				if !slices.Equal(ids, want) {
					t.Errorf("reader %d got %v, want the complete list %v", i, ids, want)

					return
				}
				results[i] = ids

				select {
				case <-clockDone:
					return
				default:
				}
			}
		}(i)
	}
	wg.Wait()
	<-clockDone

	if n := calls.Load(); n < 4 {
		t.Fatalf("logto calls = %d, want at least two refreshes: the clock must have expired the cache while the readers ran", n)
	}
	for i, ids := range results {
		if ids == nil {
			t.Fatalf("reader %d stored no result", i)
		}
		if !slices.Equal(ids, want) {
			t.Fatalf("reader %d result = %v, want %v", i, ids, want)
		}
	}

	// Reader 0 overwrites its own slice from another goroutine. If UserIDs
	// handed out the cached slice instead of a copy, every other reader's
	// result would change with it.
	var mutator sync.WaitGroup
	mutator.Go(func() {
		for k := range results[0] {
			results[0][k] = "clobbered"
		}
	})
	mutator.Wait()

	if clobbered := []string{"clobbered", "clobbered", "clobbered", "clobbered"}; !slices.Equal(results[0], clobbered) {
		t.Fatalf("the overwrite did not land: results[0] = %v", results[0])
	}
	for i, ids := range results[1:] {
		if !slices.Equal(ids, want) {
			t.Fatalf("reader %d shares reader 0's slice: %v, want %v", i+1, ids, want)
		}
	}
}
