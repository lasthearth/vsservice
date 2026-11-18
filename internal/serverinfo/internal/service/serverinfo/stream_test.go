package serverinfo

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/serverinfo/internal/model"
	"go.uber.org/zap/zapcore"
)

// streamRepo is the document the feed reads, behind the same lock the poller and
// the handlers share in production.
type streamRepo struct {
	mu    sync.Mutex
	doc   model.ServerInfo
	err   error
	reads int
}

func newStreamRepo(online, maxOnline int, worldTime string) *streamRepo {
	return &streamRepo{doc: model.ServerInfo{TotalOnline: online, MaxOnline: maxOnline, WorldTime: worldTime}}
}

func (r *streamRepo) GetServerInfo(context.Context) (*model.ServerInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.reads++
	if r.err != nil {
		return nil, r.err
	}

	doc := r.doc
	return &doc, nil
}

func (r *streamRepo) ClearStaleOnline(context.Context, int64) (int64, error) {
	return 0, nil
}

// set is one game-server write to the document.
func (r *streamRepo) set(online, maxOnline int, worldTime string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.doc = model.ServerInfo{TotalOnline: online, MaxOnline: maxOnline, WorldTime: worldTime}
}

// newStream builds the feed the way the fx graph does, then applies the tunables
// a test needs: the shipped ones are seconds long.
func newStream(t *testing.T, repo Repository, tune func(*Stream)) *Stream {
	t.Helper()

	s := NewStream(logger.NewWithCore(zapcore.NewNopCore()), repo)
	if tune != nil {
		tune(s)
	}

	return s
}

// sseFrame is one block of an event stream.
type sseFrame struct {
	event   string
	data    string
	comment string
	retry   string
}

// readFrame reads one block, which ends at the blank line. A watchdog armed by
// the caller turns a stalled stream into the read error this reports.
func readFrame(t *testing.T, r *bufio.Reader, what string) sseFrame {
	t.Helper()

	var f sseFrame
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read %s: %v (block so far: %+v)", what, err, f)
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			return f
		}

		switch {
		case strings.HasPrefix(line, ":"):
			f.comment = strings.TrimSpace(line[1:])
		case strings.HasPrefix(line, "event:"):
			f.event = strings.TrimSpace(line[len("event:"):])
		case strings.HasPrefix(line, "data:"):
			f.data = strings.TrimSpace(line[len("data:"):])
		case strings.HasPrefix(line, "retry:"):
			f.retry = strings.TrimSpace(line[len("retry:"):])
		default:
			t.Fatalf("unexpected event-stream line %q in %s", line, what)
		}
	}
}

// armWatchdog closes the body when no frame arrives in time, so a stream that
// stopped writing fails the test instead of hanging it.
func armWatchdog(t *testing.T, body io.Closer, d time.Duration) func() {
	t.Helper()

	timer := time.AfterFunc(d, func() { _ = body.Close() })
	return func() { timer.Stop() }
}

// get opens the feed and fails only on a transport error: a refusal is a status
// the caller inspects.
func get(t *testing.T, url string) *http.Response {
	t.Helper()

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}

	return resp
}

// subscriberCount reads the live subscriber count.
func subscriberCount(s *Stream) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.subs)
}

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(d)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s", d, what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// The contract the landing site consumes: the retry frame, an immediate first
// event whose payload is exactly online/max_online/time, and one more event for
// each change — and nothing in between.
func TestStreamServesFirstFrameThenEveryChange(t *testing.T) {
	repo := newStreamRepo(12, 100, "Day 45 14:20")
	s := newStream(t, repo, func(s *Stream) {
		s.pollEvery = 20 * time.Millisecond
		s.keepAlive = time.Minute
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()

	srv := httptest.NewServer(s)
	defer srv.Close()

	resp := get(t, srv.URL)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream; charset=utf-8" {
		t.Errorf("want the event-stream media type, got %q", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("want Cache-Control no-store, got %q", got)
	}
	if got := resp.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("want X-Accel-Buffering no, got %q", got)
	}

	reader := bufio.NewReader(resp.Body)
	disarm := armWatchdog(t, resp.Body, 5*time.Second)
	defer disarm()

	if retry := readFrame(t, reader, "retry frame"); retry.retry != "5000" {
		t.Fatalf("want the retry frame first, got %+v", retry)
	}

	first := readFrame(t, reader, "first event")
	if first.event != streamEvent {
		t.Fatalf("want event %q, got %+v", streamEvent, first)
	}
	const firstPayload = `{"online":12,"max_online":100,"time":"Day 45 14:20"}`
	if first.data != firstPayload {
		t.Fatalf("want data %s, got %q", firstPayload, first.data)
	}

	repo.set(13, 100, "Day 45 14:25")

	changed := readFrame(t, reader, "event after the change")
	if changed.event != streamEvent {
		t.Fatalf("want event %q, got %+v", streamEvent, changed)
	}
	const changedPayload = `{"online":13,"max_online":100,"time":"Day 45 14:25"}`
	if changed.data != changedPayload {
		t.Fatalf("want data %s, got %q", changedPayload, changed.data)
	}
}

// A connection that carries no change must not time out: the comment line is
// what keeps it open.
func TestStreamKeepAliveWithoutChange(t *testing.T) {
	repo := newStreamRepo(12, 100, "Day 45 14:20")
	s := newStream(t, repo, func(s *Stream) {
		s.keepAlive = 50 * time.Millisecond
	})
	// Seeded, not polled: Run is not started, so the only block that can follow
	// the first event is the keepalive. A change would need a poller.
	s.publish(liveInfo{Online: 12, MaxOnline: 100, Time: "Day 45 14:20"})

	srv := httptest.NewServer(s)
	defer srv.Close()

	resp := get(t, srv.URL)
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	disarm := armWatchdog(t, resp.Body, 5*time.Second)
	defer disarm()

	readFrame(t, reader, "retry frame")
	readFrame(t, reader, "first event")

	ping := readFrame(t, reader, "keepalive")
	if ping.comment != "ping" {
		t.Fatalf("want a \": ping\" comment after the keepalive interval, got %+v", ping)
	}
	if ping.event != "" || ping.data != "" {
		t.Errorf("a keepalive must not carry an event, got %+v", ping)
	}
}

// A listener at its cap must refuse with 503 and a Retry-After, not queue the
// connection: every subscriber is a goroutine and a socket.
func TestStreamRejectsSubscriberBeyondLimit(t *testing.T) {
	repo := newStreamRepo(12, 100, "Day 45 14:20")
	s := newStream(t, repo, func(s *Stream) {
		s.maxSubs = 1
		s.keepAlive = time.Minute
	})
	s.publish(liveInfo{Online: 12, MaxOnline: 100, Time: "Day 45 14:20"})

	srv := httptest.NewServer(s)
	defer srv.Close()

	first := get(t, srv.URL)
	defer first.Body.Close()

	reader := bufio.NewReader(first.Body)
	disarm := armWatchdog(t, first.Body, 5*time.Second)
	defer disarm()
	readFrame(t, reader, "retry frame")

	second := get(t, srv.URL)
	defer second.Body.Close()

	if second.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503 for the subscriber beyond the cap, got %d", second.StatusCode)
	}
	if got := second.Header.Get("Retry-After"); got != "10" {
		t.Errorf("want Retry-After 10, got %q", got)
	}
	if got := subscriberCount(s); got != 1 {
		t.Errorf("want the refused connection kept out of the hub, got %d subscribers", got)
	}
}

// A client that stops reading must be dropped, not buffered without a bound.
func TestStreamDropsSubscriberThatStopsReading(t *testing.T) {
	repo := newStreamRepo(1, 2, "Day 1 00:01")
	s := newStream(t, repo, func(s *Stream) {
		s.queue = 2
	})
	seed := liveInfo{Online: 1, MaxOnline: 2, Time: "Day 1 00:01"}
	s.publish(seed)

	ch, err := s.subscribe(context.Background())
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if got := subscriberCount(s); got != 1 {
		t.Fatalf("want 1 subscriber, got %d", got)
	}

	// Nobody drains ch: the first publish fills the last free slot, the second
	// finds the queue full.
	s.publish(liveInfo{Online: 2, MaxOnline: 2, Time: "Day 1 00:02"})
	if got := subscriberCount(s); got != 1 {
		t.Fatalf("want the subscriber kept while its queue has room, got %d", got)
	}
	s.publish(liveInfo{Online: 3, MaxOnline: 2, Time: "Day 1 00:03"})

	if got := subscriberCount(s); got != 0 {
		t.Fatalf("want the reader that never drained dropped from the hub, got %d subscribers", got)
	}

	var frames int
	for range ch {
		frames++
	}
	if frames != 2 {
		t.Errorf("want the 2 frames its queue held before the drop, got %d", frames)
	}
}

// The browser closes the tab: the request context is cancelled, the handler
// returns, and the subscriber is gone. A handler goroutine left behind would
// grow by one per connection until the process has none left.
func TestStreamReleasesSubscriberWhenClientLeaves(t *testing.T) {
	repo := newStreamRepo(12, 100, "Day 45 14:20")
	s := newStream(t, repo, func(s *Stream) {
		s.keepAlive = time.Minute
	})
	s.publish(liveInfo{Online: 12, MaxOnline: 100, Time: "Day 45 14:20"})

	before := runtime.NumGoroutine()

	for i := range 3 {
		srv := httptest.NewServer(s)

		reqCtx, cancel := context.WithCancel(context.Background())
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, srv.URL, nil)
		if err != nil {
			cancel()
			srv.Close()
			t.Fatalf("new request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			cancel()
			srv.Close()
			t.Fatalf("connection %d: %v", i, err)
		}

		reader := bufio.NewReader(resp.Body)
		readFrame(t, reader, "retry frame")
		if got := subscriberCount(s); got != 1 {
			t.Fatalf("connection %d: want 1 subscriber while the client reads, got %d", i, got)
		}

		cancel()
		resp.Body.Close()

		waitFor(t, 3*time.Second, "the subscription to be released", func() bool {
			return subscriberCount(s) == 0
		})

		srv.Close()
	}

	// A leak of one handler goroutine per connection would put this three above
	// the baseline; one above is runtime noise.
	waitFor(t, 3*time.Second, "the handler goroutines to exit", func() bool {
		return runtime.NumGoroutine() <= before+1
	})
}
