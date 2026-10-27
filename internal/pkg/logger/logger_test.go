package logger_test

import (
	"context"
	"testing"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// newObsLogger builds a logger whose records land in memory, with the
// production redaction still in the path.
func newObsLogger(t *testing.T) (logger.Logger, *observer.ObservedLogs) {
	t.Helper()

	core, logs := observer.New(zap.DebugLevel)
	return logger.NewWithCore(core), logs
}

// stringOf returns one string field of a captured record.
func stringOf(t *testing.T, e observer.LoggedEntry, key string) string {
	t.Helper()

	for _, f := range e.Context {
		if f.Key != key {
			continue
		}
		if f.Type != zapcore.StringType {
			t.Fatalf("field %q is not a string: %+v", key, f)
		}
		return f.String
	}
	t.Fatalf("record %q has no field %q: %+v", e.Message, key, e.Context)
	return ""
}

// A credential must never reach the output, whichever key spelling the call
// site used, and a normal field must survive untouched.
func TestRedactsSensitiveValues(t *testing.T) {
	l, logs := newObsLogger(t)

	l.Info("sign-in attempt",
		zap.String("access_token", "eyJhbGciOi-secret-value"),
		zap.String("client_secret", "s3cr3t"),
		zap.String("Authorization", "Bearer abc123"),
		zap.String("user_id", "uid-1"),
	)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record, got %d", len(entries))
	}
	e := entries[0]

	if got := stringOf(t, e, "access_token"); got != "[redacted]" {
		t.Errorf("access_token: want [redacted], got %q", got)
	}
	if got := stringOf(t, e, "client_secret"); got != "[redacted]" {
		t.Errorf("client_secret: want [redacted], got %q", got)
	}
	if got := stringOf(t, e, "Authorization"); got != "[redacted]" {
		t.Errorf("Authorization: want [redacted], got %q", got)
	}
	if got := stringOf(t, e, "user_id"); got != "uid-1" {
		t.Errorf("a normal field must survive redaction, got %q", got)
	}

	for _, secret := range []string{"eyJhbGciOi-secret-value", "s3cr3t", "abc123"} {
		for _, f := range e.Context {
			if f.String == secret || f.Interface == secret {
				t.Errorf("the secret %q reached the record in field %q", secret, f.Key)
			}
		}
	}
}

// Fields attached with With go through the core's With path, which is a second
// place a value can slip past a naive mask.
func TestRedactsFieldsAttachedWithWith(t *testing.T) {
	l, logs := newObsLogger(t)

	l.With(zap.String("refresh_token", "refresh-secret")).Info("later call")

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record, got %d", len(entries))
	}
	if got := stringOf(t, entries[0], "refresh_token"); got != "[redacted]" {
		t.Errorf("want [redacted], got %q", got)
	}
}

// This is the correlation gap the plan names: ids injected by the interceptors
// must appear on the records application code writes, not only on the access
// record.
func TestFromAttachesCorrelationFields(t *testing.T) {
	l, logs := newObsLogger(t)

	ctx := logging.InjectFields(
		context.Background(),
		logging.Fields{"request_id", "rid-1", "user_id", "uid-1", "method", "/m.v1.S/M"},
	)

	logger.From(ctx, l).Info("domain step")

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record, got %d", len(entries))
	}
	e := entries[0]
	for key, want := range map[string]string{
		"request_id": "rid-1",
		"user_id":    "uid-1",
		"method":     "/m.v1.S/M",
	} {
		if got := stringOf(t, e, key); got != want {
			t.Errorf("field %q: want %q, got %q", key, want, got)
		}
	}
}

// A background job has no ctx fields: From must still return a usable logger.
func TestFromFallsBackWithoutContextFields(t *testing.T) {
	l, logs := newObsLogger(t)

	logger.From(context.Background(), l).Warn("job step")

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record, got %d", len(entries))
	}
	if entries[0].Level != zapcore.WarnLevel {
		t.Errorf("want a warn record, got %v", entries[0].Level)
	}
	if hasField(entries[0], "request_id") {
		t.Error("want no request_id when the ctx carries none")
	}
}

// hasField reports whether a record carries a key at all.
func hasField(e observer.LoggedEntry, key string) bool {
	for _, f := range e.Context {
		if f.Key == key {
			return true
		}
	}
	return false
}

// A logger stored in ctx wins: a handler may attach domain fields the
// interceptors know nothing about.
func TestFromPrefersStoredLogger(t *testing.T) {
	l, logs := newObsLogger(t)

	ctx := logging.InjectFields(context.Background(), logging.Fields{"request_id", "rid-2"})
	ctx = logger.WithLogger(ctx, l.With(zap.String("entity_id", "set-9")))

	logger.From(ctx, l).Info("stored logger step")

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record, got %d", len(entries))
	}
	if got := stringOf(t, entries[0], "entity_id"); got != "set-9" {
		t.Errorf("want the stored logger's field, got %q", got)
	}
}

func TestRequestID(t *testing.T) {
	ctx := logging.InjectFields(context.Background(), logging.Fields{"request_id", "rid-3"})

	if got := logger.RequestID(ctx); got != "rid-3" {
		t.Errorf("want rid-3, got %q", got)
	}
	if got := logger.RequestID(context.Background()); got != "" {
		t.Errorf("want an empty id, got %q", got)
	}
	//nolint:staticcheck // the nil case is part of the contract: helpers are
	// called from background paths that may not carry a ctx.
	if got := logger.RequestID(nil); got != "" {
		t.Errorf("want an empty id for a nil ctx, got %q", got)
	}
}

// A nil fallback must not panic: interceptors run before every logger exists.
func TestFromWithNilFallback(t *testing.T) {
	if got := logger.From(context.Background(), nil); got != nil {
		t.Errorf("want nil, got %v", got)
	}
}

// NewWithCore must keep the redaction: a test core is not an excuse to bypass
// it.
func TestNewWithCoreRedacts(t *testing.T) {
	l, logs := newObsLogger(t)

	l.Error("request failed", zap.String("api_key", "key-value"))

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record, got %d", len(entries))
	}
	if got := stringOf(t, entries[0], "api_key"); got != "[redacted]" {
		t.Errorf("want [redacted], got %q", got)
	}
}
