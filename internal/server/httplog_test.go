package server

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// The gateway drops any header the matcher does not name, so without this the
// browser's id and the gRPC id would be two different calls in the log.
func TestForwardRequestIDHeader(t *testing.T) {
	got, ok := forwardRequestIDHeader("X-Request-Id")
	if !ok || got != interceptor.RequestIDMetadataKey {
		t.Errorf("want (%q, true), got (%q, %v)", interceptor.RequestIDMetadataKey, got, ok)
	}

	// Case-insensitive: net/http canonicalises, a proxy may not.
	got, ok = forwardRequestIDHeader("x-request-id")
	if !ok || got != interceptor.RequestIDMetadataKey {
		t.Errorf("lowercase header: want (%q, true), got (%q, %v)", interceptor.RequestIDMetadataKey, got, ok)
	}

	// Permanent headers keep the gateway default, so Authorization still
	// reaches the auth interceptor.
	got, ok = forwardRequestIDHeader("Authorization")
	if !ok || got != "grpcgateway-Authorization" {
		t.Errorf("want the default mapping for Authorization, got (%q, %v)", got, ok)
	}

	// An unknown header stays out of the metadata.
	if _, ok = forwardRequestIDHeader("X-Something-Else"); ok {
		t.Error("want an unknown header rejected")
	}
}

func TestAdoptRequestID(t *testing.T) {
	incoming := uuid.MustParse("0193f0a2-1b3c-7d4e-8f50-6172839405ab").String()

	if got := adoptRequestID(incoming); got != incoming {
		t.Errorf("want the caller id kept, got %q", got)
	}

	for _, bad := range []string{"", "not-a-uuid", strings.Repeat("a", maxRequestIDHeader+1)} {
		got := adoptRequestID(bad)
		if got == bad {
			t.Errorf("want %q replaced, got it back", bad)
		}
		if _, err := uuid.Parse(got); err != nil {
			t.Errorf("want a minted uuid for input %q, got %q: %v", bad, got, err)
		}
	}
}

// One REST call must produce a record that names the visible outcome and
// carries the same id the client can quote back.
func TestWithRequestLogEchoesIDAndRecordsOutcome(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	s := &Server{log: logger.NewWithCore(core)}

	var seenInHandler string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenInHandler = logger.RequestID(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	s.withRequestLog(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/news", nil))

	echoed := rec.Header().Get(requestIDHeader)
	if echoed == "" {
		t.Fatal("want the response to carry X-Request-Id")
	}
	if seenInHandler != echoed {
		t.Errorf("handler ctx id %q != echoed id %q", seenInHandler, echoed)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record, got %d", len(entries))
	}
	e := entries[0]
	if e.Level != zapcore.InfoLevel {
		t.Errorf("want Info, got %v", e.Level)
	}
	assertField(t, e, "event", "http.call.finish")
	assertField(t, e, "component", "gateway")
	assertField(t, e, "http_method", http.MethodGet)
	assertField(t, e, "path", "/v1/news")
	assertField(t, e, "request_id", echoed)

	status, ok := intField(e, "status")
	if !ok || status != http.StatusNoContent {
		t.Errorf("want status 204, got %+v", e.Context)
	}
	if _, ok := intField(e, "duration_ms"); !ok {
		t.Error("want a duration_ms field")
	}
}

// A client that already carries an id keeps it end to end.
func TestWithRequestLogKeepsIncomingID(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	s := &Server{log: logger.NewWithCore(core)}

	incoming := uuid.MustParse("0193f0a2-1b3c-7d4e-8f50-6172839405ab").String()
	req := httptest.NewRequest(http.MethodPost, "/logtohook", nil)
	req.Header.Set(requestIDHeader, incoming)

	rec := httptest.NewRecorder()
	s.withRequestLog(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if got := rec.Header().Get(requestIDHeader); got != incoming {
		t.Errorf("want the incoming id echoed, got %q", got)
	}
	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record, got %d", len(entries))
	}
	assertField(t, entries[0], "request_id", incoming)
}

// A websocket upgrade hijacks the connection, so the writer must stay
// unwrapped; the id and its record still apply.
func TestWithRequestLogPassesWebsocketUpgradeThrough(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	s := &Server{log: logger.NewWithCore(core)}

	req := httptest.NewRequest(http.MethodGet, "/v1/notifications/stream", nil)
	// Both headers: this is what the proxy outside this middleware requires,
	// and the predicate here delegates to the same gorilla check.
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")

	var wrapped bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, wrapped = w.(*statusRecorder)
		w.Header().Set("Upgrade", "websocket")
		w.WriteHeader(http.StatusSwitchingProtocols)
	})

	rec := httptest.NewRecorder()
	s.withRequestLog(inner).ServeHTTP(rec, req)

	if wrapped {
		t.Error("an upgrade request must not get a wrapped writer")
	}
	if rec.Header().Get(requestIDHeader) == "" {
		t.Error("want the id echoed for an upgrade too")
	}
	if n := logs.Len(); n != 0 {
		t.Errorf("want no http record for an upgrade, got %d", n)
	}
}

// An Upgrade header the proxy does not accept is a plain HTTP request and must
// be recorded: the old header-only predicate dropped the access record for it.
func TestWithRequestLogRecordsUpgradeHeaderWithoutConnection(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	s := &Server{log: logger.NewWithCore(core)}

	req := httptest.NewRequest(http.MethodPost, "/v1/tickets/abc", nil)
	req.Header.Set("Upgrade", "websocket")

	rec := httptest.NewRecorder()
	s.withRequestLog(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})).ServeHTTP(rec, req)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record for a request the proxy did not take over, got %d", len(entries))
	}
	assertField(t, entries[0], "event", "http.call.finish")
	assertField(t, entries[0], "path", "/v1/tickets/abc")
	status, ok := intField(entries[0], "status")
	if !ok || status != http.StatusCreated {
		t.Errorf("want status 201, got %+v", entries[0].Context)
	}
}

// The recorder must not break flushing or hijacking for handlers that need it.
func TestStatusRecorderUnwrap(t *testing.T) {
	inner := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: inner}

	if got := rec.Unwrap(); got != http.ResponseWriter(inner) {
		t.Error("Unwrap must return the wrapped writer")
	}

	n, err := rec.Write([]byte("body"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != 4 {
		t.Errorf("want 4 bytes written, got %d", n)
	}
	if rec.status != http.StatusOK {
		t.Errorf("an implicit write is a 200, got %d", rec.status)
	}
	rec.Flush()
	if !inner.Flushed {
		t.Error("Flush must reach the wrapped writer")
	}
}

// A handler that writes nothing still produces a record: a 200 is what the
// client saw.
func TestWithRequestLogDefaultsStatusToOK(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	s := &Server{log: logger.NewWithCore(core)}

	rec := httptest.NewRecorder()
	s.withRequestLog(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/quiet", nil))

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record, got %d", len(entries))
	}
	status, ok := intField(entries[0], "status")
	if !ok || status != http.StatusOK {
		t.Errorf("want status 200, got %v (%v)", status, ok)
	}
}

// An event-stream route answers once and then holds the response open, so its
// record must not report duration_ms: the field every other route fills with
// milliseconds would carry minutes or hours, and any duration dashboard built on
// it would be unreadable. The frame must also reach the client before the
// handler returns — a buffered stream is a frozen widget.
func TestWithRequestLogRecordsStreamWithoutDuration(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	s := &Server{log: logger.NewWithCore(core)}

	delivered := make(chan struct{})

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		rc := http.NewResponseController(w)
		if _, err := w.Write([]byte("retry: 5000\n\n")); err != nil {
			t.Errorf("write: %v", err)
			return
		}
		if err := rc.Flush(); err != nil {
			t.Errorf("flush through the recorder: %v", err)
			return
		}

		// The handler keeps the response open until the client has the frame.
		// If the writer swallowed it, this fails rather than passing on a
		// buffer drained at the end.
		select {
		case <-delivered:
		case <-time.After(5 * time.Second):
			t.Error("the frame never reached the client while the handler held the response")
		}
	})

	srv := httptest.NewServer(s.withRequestLog(inner))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil {
		t.Fatalf("read the first line: %v", err)
	}
	if line != "retry: 5000\n" {
		t.Errorf("want the first frame, got %q", line)
	}
	close(delivered)

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatalf("drain the stream: %v", err)
	}

	// The record is written after the handler returns, which is a moment after
	// the client sees the end of the body.
	deadline := time.Now().Add(2 * time.Second)
	for logs.Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record, got %d", len(entries))
	}
	e := entries[0]
	if e.Level != zapcore.InfoLevel {
		t.Errorf("want Info, got %v", e.Level)
	}
	assertField(t, e, "event", "http.call.stream.finish")
	assertField(t, e, "component", "gateway")
	assertField(t, e, "http_method", http.MethodGet)

	if _, ok := intField(e, "duration_ms"); ok {
		t.Errorf("a stream must not report duration_ms, got %+v", e.Context)
	}
	if _, ok := intField(e, "connection_ms"); !ok {
		t.Errorf("want a connection_ms field for a stream, got %+v", e.Context)
	}
}

// A refusal is a short answer, not a stream: the 503 a full or unreadable feed
// sends replaces the media type, so it must land in the ordinary record.
func TestWithRequestLogRecordsStreamRefusalAsCall(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	s := &Server{log: logger.NewWithCore(core)}

	rec := httptest.NewRecorder()
	s.withRequestLog(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Retry-After", "10")
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/serverinfo/stream", nil))

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record, got %d", len(entries))
	}
	assertField(t, entries[0], "event", "http.call.finish")
	if _, ok := intField(entries[0], "duration_ms"); !ok {
		t.Error("want duration_ms on a refused request")
	}
}

// The id must be in the request context as a middleware field, because that is
// what logger.From reads.
func TestWithRequestLogInjectsMiddlewareField(t *testing.T) {
	core, _ := observer.New(zapcore.DebugLevel)
	s := &Server{log: logger.NewWithCore(core)}

	var fields logging.Fields
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		fields = logging.ExtractFields(r.Context())
	})

	rec := httptest.NewRecorder()
	s.withRequestLog(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/x", nil))

	if len(fields) != 2 || fields[0] != "request_id" {
		t.Errorf("want a request_id middleware field, got %v", fields)
	}
	if got, ok := fields[1].(string); !ok || got != rec.Header().Get(requestIDHeader) {
		t.Errorf("want the field to equal the echoed id, got %v", fields[1])
	}
}

func assertField(t *testing.T, e observer.LoggedEntry, key, want string) {
	t.Helper()

	for _, f := range e.Context {
		if f.Key == key {
			if f.Type != zapcore.StringType || f.String != want {
				t.Errorf("field %q: want %q, got %+v", key, want, f)
			}
			return
		}
	}
	t.Errorf("record %q has no field %q: %+v", e.Message, key, e.Context)
}

func intField(e observer.LoggedEntry, key string) (int64, bool) {
	for _, f := range e.Context {
		if f.Key != key {
			continue
		}
		switch f.Type {
		case zapcore.Int64Type, zapcore.Int32Type, zapcore.Int16Type, zapcore.Int8Type:
			return f.Integer, true
		default:
			return 0, false
		}
	}
	return 0, false
}
