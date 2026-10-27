// Package logtest adapts a zap observer core to the project logger interface,
// so messaging tests assert on the exact records a bus emits instead of parsing
// output. It mirrors the adapter the server interceptors test with.
package logtest

import (
	"context"
	"testing"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// Observed is a logger whose records an observer captures. It satisfies
// logger.Logger, so a bus under test treats it like the real one.
type Observed struct {
	*zap.Logger
}

// New returns a logger and the log sink its records go to. Everything from
// Debug up is captured: a missing record is a failure, not silence.
func New() (logger.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zap.DebugLevel)
	return Observed{Logger: zap.New(core)}, logs
}

func (l Observed) With(fields ...zap.Field) logger.Logger {
	return Observed{Logger: l.Logger.With(fields...)}
}

func (l Observed) Named(name string) logger.Logger {
	return Observed{Logger: l.Logger.Named(name)}
}

func (l Observed) WithScope(scope string) logger.Logger {
	return l.With(zap.String("scope", scope))
}

func (l Observed) WithMethod(method string) logger.Logger {
	return l.With(zap.String("method", method))
}

func (l Observed) WithComponent(component string) logger.Logger {
	return l.With(zap.String("component", component))
}

func (l Observed) WithOptions(opts ...zap.Option) logger.Logger {
	return Observed{Logger: l.Logger.WithOptions(opts...)}
}

// ToLoggingLogger is unused by the buses under test.
func (l Observed) ToLoggingLogger() logging.Logger { return nil }

func (l Observed) Sync() error { return l.Logger.Sync() }

// CtxWithRequestID returns a ctx carrying the correlation field the gRPC
// middleware injects, so a publish-side test sees the same ctx a handler
// passes.
func CtxWithRequestID(requestID string) context.Context {
	return logging.InjectFields(
		context.Background(),
		logging.Fields{"request_id", requestID},
	)
}

// Entry returns the first record with message, failing the test when none was
// logged: a test that passes on silence proves nothing.
func Entry(t *testing.T, logs *observer.ObservedLogs, message string) observer.LoggedEntry {
	t.Helper()
	entries := logs.FilterMessage(message).All()
	if len(entries) == 0 {
		t.Fatalf("no record %q was logged; got %v", message, logs.All())
	}
	return entries[0]
}

// NoEntry fails the test when a record with message was logged.
func NoEntry(t *testing.T, logs *observer.ObservedLogs, message string) {
	t.Helper()
	if entries := logs.FilterMessage(message).All(); len(entries) > 0 {
		t.Fatalf("record %q was logged, want none", message)
	}
}

// Field returns one field of a record.
func Field(e observer.LoggedEntry, key string) (zap.Field, bool) {
	for _, f := range e.Context {
		if f.Key == key {
			return f, true
		}
	}
	return zap.Field{}, false
}

// StringField returns one string field of a record, failing the test when it is
// absent or of another type.
func StringField(t *testing.T, e observer.LoggedEntry, key string) string {
	t.Helper()
	f, ok := Field(e, key)
	if !ok {
		t.Fatalf("record %q has no field %q: %+v", e.Message, key, e.Context)
	}
	if f.Type != zapcore.StringType {
		t.Fatalf("field %q of %q is not a string: %+v", key, e.Message, f)
	}
	return f.String
}

// IntField returns one int field of a record, failing the test when it is
// absent or of another type.
func IntField(t *testing.T, e observer.LoggedEntry, key string) int {
	t.Helper()
	f, ok := Field(e, key)
	if !ok {
		t.Fatalf("record %q has no field %q: %+v", e.Message, key, e.Context)
	}
	if f.Type != zapcore.Int64Type {
		t.Fatalf("field %q of %q is not an int: %+v", key, e.Message, f)
	}
	return int(f.Integer)
}

// Uint64Field returns one uint64 field of a record, failing the test when it is
// absent or of another type.
func Uint64Field(t *testing.T, e observer.LoggedEntry, key string) uint64 {
	t.Helper()
	f, ok := Field(e, key)
	if !ok {
		t.Fatalf("record %q has no field %q: %+v", e.Message, key, e.Context)
	}
	if f.Type != zapcore.Uint64Type {
		t.Fatalf("field %q of %q is not a uint64: %+v", key, e.Message, f)
	}
	// zap stores every integer field in the signed Integer slot.
	return uint64(f.Integer)
}

// ErrorField returns the error field of a record, failing the test when absent.
func ErrorField(t *testing.T, e observer.LoggedEntry) error {
	t.Helper()
	f, ok := Field(e, "error")
	if !ok {
		t.Fatalf("record %q has no error field: %+v", e.Message, e.Context)
	}
	return f.Interface.(error)
}

// NoPayloadFields fails the test when any captured record carries the message
// payload. Only its size is safe to log.
func NoPayloadFields(t *testing.T, logs *observer.ObservedLogs) {
	t.Helper()
	for _, e := range logs.All() {
		for _, f := range e.Context {
			switch f.Key {
			case "payload", "data", "body", "event", "message_body":
				t.Fatalf("record %q carries a payload field %q", e.Message, f.Key)
			}
		}
	}
}

// Levels returns the level of every record with message, so a test can assert
// that a failure was logged at Warn instead of Error.
func Levels(logs *observer.ObservedLogs, message string) []zapcore.Level {
	entries := logs.FilterMessage(message).All()
	levels := make([]zapcore.Level, 0, len(entries))
	for _, e := range entries {
		levels = append(levels, e.Level)
	}
	return levels
}
