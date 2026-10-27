package httpclient

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// A Telegram bot token lives in the URL path, so masking by field key alone
// would leak it.
func TestMaskSecrets(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "telegram bot token in path",
			in:   "/bot123456789:AAHdqTcvCH1vGJHFsiiretUqLbP3XyZZ9/sendMessage",
			want: "/bot[redacted]/sendMessage",
		},
		{
			name: "token in query",
			in:   "https://example.com/hook?token=abc123def456",
			want: "https://example.com/hook?token=[redacted]",
		},
		{
			name: "api_key in query with another parameter after it",
			in:   "/v1/send?api_key=sekret&chat_id=42",
			want: "/v1/send?api_key=[redacted]&chat_id=42",
		},
		{
			name: "ordinary path untouched",
			in:   "/api/v10/users/@me",
			want: "/api/v10/users/@me",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskSecrets(tt.in); got != tt.want {
				t.Errorf("want %q, got %q", tt.want, got)
			}
		})
	}
}

func TestKVFieldMasksStrings(t *testing.T) {
	f := kvField("url", "https://api.telegram.org/bot123456789:AAHdqTcvCH1vGJHFsiiretUqLbP3XyZZ9/sendMessage")
	if f.Type != zapcore.StringType {
		t.Fatalf("want a string field, got %+v", f)
	}
	if strings.Contains(f.String, "AAHdqTcvCH1vGJHFsiiretUqLbP3XyZZ9") {
		t.Errorf("the token reached the field: %q", f.String)
	}

	// Errors keep their text: they are the debugging signal, and a driver or
	// transport error does not embed the credential.
	cause := errors.New("dial tcp: connection refused")
	if got := kvField("error", cause); got.Type != zapcore.ErrorType {
		t.Errorf("want an error field, got %+v", got)
	}
}

func TestRequestHookRecordsTheAttempt(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	log := logger.NewWithCore(core)

	req := httptest.NewRequest(http.MethodPost,
		"https://api.telegram.org/bot123456789:AAHdqTcvCH1vGJHFsiiretUqLbP3XyZZ9/sendMessage", nil)

	requestHook(log)(nil, req, 1)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record, got %d", len(entries))
	}
	e := entries[0]
	assertString(t, e, "event", "http.out")
	assertString(t, e, "component", "http")
	assertString(t, e, "http_method", http.MethodPost)
	assertString(t, e, "host", "api.telegram.org")
	assertString(t, e, "path", "/bot[redacted]/sendMessage")
	assertInt(t, e, "attempt", 2)

	if strings.Contains(e.Message, "AAHdqTcvCH1vGJHFsiiretUqLbP3XyZZ9") {
		t.Error("the token reached the record message")
	}
}

func TestResponseHookRecordsTheStatus(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	log := logger.NewWithCore(core)

	req := httptest.NewRequest(http.MethodGet, "https://discord.com/api/v10/users/@me", nil)
	resp := &http.Response{StatusCode: http.StatusTooManyRequests, Request: req}

	responseHook(log)(nil, resp)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record, got %d", len(entries))
	}
	assertString(t, entries[0], "event", "http.out.response")
	assertInt(t, entries[0], "status", http.StatusTooManyRequests)
}

// The final failure after five silent retries is the record that explains a
// timeout the user saw.
func TestErrorHookRecordsTheFailureAndClosesTheBody(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	log := logger.NewWithCore(core)

	closed := false
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Request:    httptest.NewRequest(http.MethodGet, "https://example.com/v1/x", nil),
		Body:       &closeTracker{onClose: func() { closed = true }},
	}

	gotResp, gotErr := errorHook(log)(resp, errors.New("giving up after 5 attempts"), 5)

	if gotResp != resp || gotErr == nil {
		t.Errorf("want the response and error passed through, got (%v, %v)", gotResp, gotErr)
	}
	if !closed {
		t.Error("the hook must close the body it abandons")
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record, got %d", len(entries))
	}
	assertString(t, entries[0], "event", "http.out.failed")
	assertInt(t, entries[0], "tries", 5)
}

// With logging off the library's own stderr logger must stay silent, or retry
// noise bypasses the structured stream.
func TestNewDisablesLibraryLoggingWhenOff(t *testing.T) {
	core, _ := observer.New(zapcore.DebugLevel)
	log := logger.NewWithCore(core)

	client := New(log, config.Config{HTTPClientLog: false})
	if client == nil {
		t.Fatal("want a usable client")
	}
}

func TestNewReturnsAUsableClientWhenOn(t *testing.T) {
	core, _ := observer.New(zapcore.DebugLevel)
	log := logger.NewWithCore(core)

	client := New(log, config.Config{HTTPClientLog: true})
	if client == nil {
		t.Fatal("want a usable client")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := client.Get(srv.URL + "/v1/ping")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

// closeTracker reports whether a response body was closed.
type closeTracker struct {
	onClose func()
}

func (c *closeTracker) Read([]byte) (int, error) { return 0, errors.New("eof") }

func (c *closeTracker) Close() error {
	c.onClose()
	return nil
}

func assertString(t *testing.T, e observer.LoggedEntry, key, want string) {
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

func assertInt(t *testing.T, e observer.LoggedEntry, key string, want int) {
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
