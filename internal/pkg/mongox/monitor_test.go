package mongox

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// obsLogger adapts a *zap.Logger to logger.Logger, so the monitor can be
// driven against a zaptest/observer core without a live server.
type obsLogger struct{ *zap.Logger }

var _ logger.Logger = obsLogger{}

func (l obsLogger) With(fields ...zap.Field) logger.Logger {
	return obsLogger{l.Logger.With(fields...)}
}

func (l obsLogger) Named(name string) logger.Logger { return obsLogger{l.Logger.Named(name)} }

func (l obsLogger) WithScope(scope string) logger.Logger {
	return l.With(zap.String("scope", scope))
}

func (l obsLogger) WithMethod(method string) logger.Logger {
	return l.With(zap.String("method", method))
}

func (l obsLogger) WithComponent(component string) logger.Logger {
	return l.With(zap.String("component", component))
}

func (l obsLogger) WithOptions(opts ...zap.Option) logger.Logger {
	return obsLogger{l.Logger.WithOptions(opts...)}
}

// ToLoggingLogger returns nil because the monitor never asks for the
// grpc-middleware adapter.
func (l obsLogger) ToLoggingLogger() logging.Logger { return nil }

func (l obsLogger) Sync() error { return l.Logger.Sync() }

func newTestMonitor(t *testing.T, slowMS int64) (*commandMonitor, *observer.ObservedLogs) {
	t.Helper()

	core, logs := observer.New(zapcore.DebugLevel)
	return newCommandMonitor(obsLogger{zap.New(core)}, slowMS), logs
}

// rawCommand marshals a command document the way the driver hands one to the
// started callback.
func rawCommand(t *testing.T, doc bson.D) bson.Raw {
	t.Helper()

	b, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal command document: %v", err)
	}
	return bson.Raw(b)
}

// findCommand carries a marker value in the filter. Any record that contains
// the marker proves the monitor leaked user content into the log.
const secretFilterValue = "hunter2-session-token"

func findCommand(t *testing.T) bson.Raw {
	t.Helper()

	return rawCommand(t, bson.D{
		{Key: "find", Value: "mods"},
		{Key: "filter", Value: bson.D{{Key: "token", Value: secretFilterValue}}},
		{Key: "limit", Value: 10},
	})
}

func startedEvent(requestID int64, commandName string, cmd bson.Raw) *event.CommandStartedEvent {
	return &event.CommandStartedEvent{
		Command:      cmd,
		DatabaseName: "lsp",
		CommandName:  commandName,
		RequestID:    requestID,
	}
}

func succeededEvent(requestID int64, commandName string, d time.Duration) *event.CommandSucceededEvent {
	return &event.CommandSucceededEvent{
		CommandFinishedEvent: finishedEvent(requestID, commandName, d),
	}
}

func failedEvent(requestID int64, commandName string, d time.Duration, failure error) *event.CommandFailedEvent {
	return &event.CommandFailedEvent{
		CommandFinishedEvent: finishedEvent(requestID, commandName, d),
		Failure:              failure,
	}
}

func finishedEvent(requestID int64, commandName string, d time.Duration) event.CommandFinishedEvent {
	return event.CommandFinishedEvent{
		Duration:     d,
		CommandName:  commandName,
		DatabaseName: "lsp",
		RequestID:    requestID,
	}
}

func assertField(t *testing.T, entry observer.LoggedEntry, key string, want any) {
	t.Helper()

	fields := entry.ContextMap()
	got, ok := fields[key]
	if !ok {
		t.Fatalf("record %q has no field %q, fields: %v", entry.Message, key, fields)
	}
	if got != want {
		t.Fatalf("record %q field %q = %#v, want %#v", entry.Message, key, got, want)
	}
}

func assertNoField(t *testing.T, entry observer.LoggedEntry, key string) {
	t.Helper()

	if got, ok := entry.ContextMap()[key]; ok {
		t.Fatalf("record %q carries field %q = %#v, want it omitted", entry.Message, key, got)
	}
}

func TestCommandMonitorFailedCommand(t *testing.T) {
	m, logs := newTestMonitor(t, 250)
	ctx := context.Background()

	m.started(ctx, startedEvent(1, "find", findCommand(t)))
	m.failed(ctx, failedEvent(1, "find", 12*time.Millisecond, errors.New("context deadline exceeded")))

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("records = %d, want 1: %v", len(entries), entries)
	}

	entry := entries[0]
	if entry.Level != zapcore.ErrorLevel {
		t.Fatalf("level = %v, want %v", entry.Level, zapcore.ErrorLevel)
	}
	assertField(t, entry, "event", eventCommandFailed)
	assertField(t, entry, "component", componentMongo)
	assertField(t, entry, "command_name", "find")
	assertField(t, entry, "database_name", "lsp")
	assertField(t, entry, "collection_name", "mods")
	assertField(t, entry, "duration_ms", int64(12))
	assertField(t, entry, "error", "context deadline exceeded")

	m.mu.Lock()
	pending := len(m.pending)
	m.mu.Unlock()
	if pending != 0 {
		t.Fatalf("pending entries = %d, want 0 after a failure", pending)
	}
}

func TestCommandMonitorSlowCommand(t *testing.T) {
	m, logs := newTestMonitor(t, 250)
	ctx := context.Background()

	m.started(ctx, startedEvent(2, "find", findCommand(t)))
	m.succeeded(ctx, succeededEvent(2, "find", 300*time.Millisecond))

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("records = %d, want 1: %v", len(entries), entries)
	}

	entry := entries[0]
	if entry.Level != zapcore.WarnLevel {
		t.Fatalf("level = %v, want %v", entry.Level, zapcore.WarnLevel)
	}
	assertField(t, entry, "event", eventCommandSlow)
	assertField(t, entry, "component", componentMongo)
	assertField(t, entry, "command_name", "find")
	assertField(t, entry, "database_name", "lsp")
	assertField(t, entry, "collection_name", "mods")
	assertField(t, entry, "duration_ms", int64(300))
	assertNoField(t, entry, "error")
}

func TestCommandMonitorFastCommandLogsNothing(t *testing.T) {
	m, logs := newTestMonitor(t, 250)
	ctx := context.Background()

	m.started(ctx, startedEvent(3, "find", findCommand(t)))
	m.succeeded(ctx, succeededEvent(3, "find", 5*time.Millisecond))

	// The exact threshold is slow, not fast: the check is >=.
	m.started(ctx, startedEvent(4, "find", findCommand(t)))
	m.succeeded(ctx, succeededEvent(4, "find", 250*time.Millisecond))

	if got := logs.Len(); got != 1 {
		t.Fatalf("records = %d, want 1 from the 250ms command only", got)
	}
	assertField(t, logs.All()[0], "duration_ms", int64(250))

	m.mu.Lock()
	pending := len(m.pending)
	m.mu.Unlock()
	if pending != 0 {
		t.Fatalf("pending entries = %d, want 0 after both commands finished", pending)
	}
}

func TestCommandMonitorSlowThresholdDisabled(t *testing.T) {
	for _, slowMS := range []int64{0, -1} {
		m, logs := newTestMonitor(t, slowMS)
		ctx := context.Background()

		m.started(ctx, startedEvent(5, "aggregate", rawCommand(t, bson.D{{Key: "aggregate", Value: "mods"}, {Key: "pipeline", Value: secretFilterValue}})))
		m.succeeded(ctx, succeededEvent(5, "aggregate", 30*time.Second))

		if got := logs.Len(); got != 0 {
			t.Fatalf("slowMS = %d: records = %d, want 0", slowMS, got)
		}
	}
}

func TestCommandMonitorCollectionOmittedForNonCrudCommand(t *testing.T) {
	m, logs := newTestMonitor(t, 250)
	ctx := context.Background()

	m.started(ctx, startedEvent(6, "ping", rawCommand(t, bson.D{{Key: "ping", Value: 1}})))
	m.failed(ctx, failedEvent(6, "ping", time.Millisecond, errors.New("connection reset")))

	// A database-level aggregate has no collection either: its first field
	// holds 1, not a name.
	m.started(ctx, startedEvent(7, "aggregate", rawCommand(t, bson.D{{Key: "aggregate", Value: 1}, {Key: "pipeline", Value: bson.A{}}})))
	m.succeeded(ctx, succeededEvent(7, "aggregate", 400*time.Millisecond))

	entries := logs.All()
	if len(entries) != 2 {
		t.Fatalf("records = %d, want 2: %v", len(entries), entries)
	}
	for _, entry := range entries {
		assertNoField(t, entry, "collection_name")
	}
	assertField(t, entries[0], "command_name", "ping")
	assertField(t, entries[1], "command_name", "aggregate")
}

func TestCommandMonitorNeverLogsCommandDocument(t *testing.T) {
	m, logs := newTestMonitor(t, 1)
	ctx := context.Background()

	m.started(ctx, startedEvent(8, "find", findCommand(t)))
	m.failed(ctx, failedEvent(8, "find", 2*time.Millisecond, errors.New("write conflict")))
	m.started(ctx, startedEvent(9, "update", rawCommand(t, bson.D{
		{Key: "update", Value: "mods"},
		{Key: "updates", Value: bson.A{bson.D{{Key: "q", Value: secretFilterValue}, {Key: "u", Value: secretFilterValue}}}},
	})))
	m.succeeded(ctx, succeededEvent(9, "update", 2*time.Millisecond))

	entries := logs.All()
	if len(entries) != 2 {
		t.Fatalf("records = %d, want 2: %v", len(entries), entries)
	}

	for _, entry := range entries {
		if strings.Contains(entry.Message, secretFilterValue) {
			t.Fatalf("msg %q leaks the command document", entry.Message)
		}
		for key, value := range entry.ContextMap() {
			if strings.Contains(key, secretFilterValue) || strings.Contains(fmt.Sprint(value), secretFilterValue) {
				t.Fatalf("field %q = %#v leaks the command document", key, value)
			}
		}
	}
}

func TestCommandMonitorCarriesRequestIDFromContext(t *testing.T) {
	m, logs := newTestMonitor(t, 250)

	// logger.From reads the ids the interceptors put in ctx, so the monitor
	// must pass the driver ctx through instead of logging on the base logger.
	ctx := logging.InjectFields(context.Background(), logging.Fields{
		"request_id", "01931f00-0000-7000-8000-000000000000",
	})

	m.started(ctx, startedEvent(10, "find", findCommand(t)))
	m.failed(ctx, failedEvent(10, "find", time.Millisecond, errors.New("boom")))

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("records = %d, want 1: %v", len(entries), entries)
	}
	assertField(t, entries[0], "request_id", "01931f00-0000-7000-8000-000000000000")
}

func TestCommandMonitorCapsPendingEntries(t *testing.T) {
	m, logs := newTestMonitor(t, 250)
	ctx := context.Background()

	// None of these commands finishes, which is the leak the cap bounds.
	total := int64(maxPendingCommands + 10)
	for id := int64(0); id < total; id++ {
		m.started(ctx, startedEvent(id, "ping", rawCommand(t, bson.D{{Key: "ping", Value: 1}})))
	}

	m.mu.Lock()
	size := len(m.pending)
	ordered := m.order.Len()
	_, oldestKept := m.pending[0]
	_, newestKept := m.pending[total-1]
	m.mu.Unlock()

	if size > maxPendingCommands {
		t.Fatalf("pending = %d, want at most %d", size, maxPendingCommands)
	}
	if ordered != size {
		t.Fatalf("order length %d and pending length %d disagree", ordered, size)
	}
	if oldestKept {
		t.Fatal("eviction kept the oldest entry")
	}
	if !newestKept {
		t.Fatal("eviction dropped the newest entry")
	}
	if got := logs.Len(); got != 0 {
		t.Fatalf("started events produced %d records, want 0", got)
	}
}

func TestCommandMonitorConcurrentCommands(t *testing.T) {
	m, logs := newTestMonitor(t, 1)
	ctx := context.Background()
	cmd := findCommand(t)

	const workers = 8
	const perWorker = 50

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				id := int64(worker*perWorker + i)
				m.started(ctx, startedEvent(id, "find", cmd))
				m.succeeded(ctx, succeededEvent(id, "find", 2*time.Millisecond))
			}
		}(w)
	}
	wg.Wait()

	// Every command finished, so every one is slow at a 1ms threshold and
	// nothing stays in the correlation map.
	if got, want := logs.Len(), workers*perWorker; got != want {
		t.Fatalf("records = %d, want %d", got, want)
	}
	m.mu.Lock()
	pending, ordered := len(m.pending), m.order.Len()
	m.mu.Unlock()
	if pending != 0 || ordered != 0 {
		t.Fatalf("pending = %d, order = %d, want both 0", pending, ordered)
	}
}
