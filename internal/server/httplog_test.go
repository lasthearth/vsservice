package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
