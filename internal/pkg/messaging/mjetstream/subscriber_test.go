package mjetstream

import (
	"context"
	"errors"
	"testing"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/lasthearth/vsservice/internal/pkg/messaging/logtest"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap/zapcore"
)

// newTestSubscriber builds a subscriber without a JetStream context: the
// message handling is the traced part and it never touches the server.
func newTestSubscriber(log logger.Logger, maxDeliverLog uint64) *Subscriber[testEvent] {
	if maxDeliverLog == 0 {
		maxDeliverLog = messaging.DefaultMaxDeliverLog
	}
	return &Subscriber[testEvent]{
		stream:   testStream,
		subject:  testSubject,
		consName: testConsumer,
		// Tagged is what NewSubscriber does, so the records carry the component.
		log:           messaging.Tagged(log),
		maxDeliverLog: maxDeliverLog,
	}
}

// deliveredMsg models one delivery.
func deliveredMsg(payload []byte, requestID string, numDelivered uint64) *fakeMsg {
	return &fakeMsg{
		subject: testSubject,
		data:    payload,
		headers: messaging.OutgoingHeader(logtest.CtxWithRequestID(requestID), true),
		meta: &jetstream.MsgMetadata{
			Stream:       testStream,
			Consumer:     testConsumer,
			NumDelivered: numDelivered,
		},
	}
}

// ctxDelivered reads num_delivered back out of a handler ctx.
func ctxDelivered(ctx context.Context) (uint64, bool) {
	fields := logging.ExtractFields(ctx)
	for i := 0; i+1 < len(fields); i += 2 {
		key, ok := fields[i].(string)
		if !ok || key != "num_delivered" {
			continue
		}
		if got, ok := fields[i+1].(uint64); ok {
			return got, true
		}
	}
	return 0, false
}

func TestHandleMessageInjectsCorrelationAndAcks(t *testing.T) {
	log, logs := logtest.New()
	s := newTestSubscriber(log, 0)
	msg := deliveredMsg(testPayload, "req-41", 1)

	var gotRID string
	var gotDelivered uint64
	var gotEvent testEvent
	s.handleMessage(context.Background(), msg,
		func(ctx context.Context, event testEvent) error {
			// The id and the delivery attempt must reach the handler ctx, so
			// its own records show the same correlation.
			gotRID = logger.RequestID(ctx)
			gotDelivered, _ = ctxDelivered(ctx)
			gotEvent = event
			return nil
		})

	if gotRID != "req-41" {
		t.Fatalf("handler ctx request_id = %q, want %q", gotRID, "req-41")
	}
	if gotDelivered != 1 {
		t.Fatalf("handler ctx num_delivered = %d, want 1", gotDelivered)
	}
	if gotEvent.KitID != "k1" || gotEvent.PlayerID != "p1" {
		t.Fatalf("handler got %+v, want the decoded payload", gotEvent)
	}
	if msg.acks != 1 || msg.naks != 0 {
		t.Fatalf("acks = %d, naks = %d, want one ack", msg.acks, msg.naks)
	}

	start := logtest.Entry(t, logs, messaging.EventConsumeStart)
	if start.Level != zapcore.DebugLevel {
		t.Fatalf("consume.start level = %v, want debug", start.Level)
	}
	if got := logtest.StringField(t, start, "stream"); got != testStream {
		t.Fatalf("stream = %q, want %q", got, testStream)
	}
	if got := logtest.StringField(t, start, "consumer"); got != testConsumer {
		t.Fatalf("consumer = %q, want %q", got, testConsumer)
	}
	if got := logtest.StringField(t, start, "subject"); got != testSubject {
		t.Fatalf("subject = %q, want %q", got, testSubject)
	}
	if got := logtest.IntField(t, start, "payload_bytes"); got != len(testPayload) {
		t.Fatalf("payload_bytes = %d, want %d", got, len(testPayload))
	}
	if got := logtest.Uint64Field(t, start, "num_delivered"); got != 1 {
		t.Fatalf("num_delivered = %d, want 1", got)
	}
	if got := logtest.StringField(t, start, "component"); got != messaging.Component {
		t.Fatalf("component = %q, want %q", got, messaging.Component)
	}

	ok := logtest.Entry(t, logs, messaging.EventConsumeOK)
	if got := logtest.StringField(t, ok, "request_id"); got != "req-41" {
		t.Fatalf("request_id = %q, want %q", got, "req-41")
	}

	logtest.NoEntry(t, logs, messaging.EventConsumeFailed)
	logtest.NoEntry(t, logs, messaging.EventAckFailed)
	logtest.NoPayloadFields(t, logs)
}

func TestHandleMessageWithoutHeaderKeepsWorking(t *testing.T) {
	log, logs := logtest.New()
	s := newTestSubscriber(log, 0)
	// A publisher on an old server sends no header: the delivery is still
	// processed and no id is invented.
	msg := deliveredMsg(testPayload, "", 1)
	msg.headers = nil

	var gotRID = "unset"
	s.handleMessage(context.Background(), msg,
		func(ctx context.Context, _ testEvent) error {
			gotRID = logger.RequestID(ctx)
			return nil
		})

	if gotRID != "" {
		t.Fatalf("handler ctx request_id = %q, want empty", gotRID)
	}
	entry := logtest.Entry(t, logs, messaging.EventConsumeOK)
	if _, has := logtest.Field(entry, "request_id"); has {
		t.Fatalf("record carries a request_id it cannot know: %+v", entry.Context)
	}
	if msg.acks != 1 {
		t.Fatalf("acks = %d, want one", msg.acks)
	}
}

func TestHandleMessageWarnsOnKnownRedelivery(t *testing.T) {
	log, logs := logtest.New()
	// The default threshold is 3, so the third delivery is a known retrying
	// message.
	s := newTestSubscriber(log, 0)
	msg := deliveredMsg(testPayload, "req-42", 3)

	handlerErr := errors.New("repo down")
	s.handleMessage(context.Background(), msg,
		func(context.Context, testEvent) error { return handlerErr })

	entries := logs.FilterMessage(messaging.EventConsumeFailed).All()
	if len(entries) != 1 {
		t.Fatalf("captured %d consume.failed records, want 1: %v", len(entries), entries)
	}
	entry := entries[0]
	if entry.Level != zapcore.WarnLevel {
		t.Fatalf("level = %v, want warn for a message at the redelivery threshold", entry.Level)
	}
	if got := logtest.ErrorField(t, entry); !errors.Is(got, handlerErr) {
		t.Fatalf("error = %v, want %v", got, handlerErr)
	}
	if got := logtest.Uint64Field(t, entry, "num_delivered"); got != 3 {
		t.Fatalf("num_delivered = %d, want 3", got)
	}
	if msg.naks != 1 {
		t.Fatalf("naks = %d, want one", msg.naks)
	}

	logtest.NoEntry(t, logs, messaging.EventConsumeOK)
	logtest.NoPayloadFields(t, logs)
}

func TestHandleMessageErrorsOnEarlyFailure(t *testing.T) {
	log, logs := logtest.New()
	s := newTestSubscriber(log, 0)
	msg := deliveredMsg(testPayload, "req-43", 1)

	s.handleMessage(context.Background(), msg,
		func(context.Context, testEvent) error { return errors.New("handler failed") })

	entry := logtest.Entry(t, logs, messaging.EventConsumeFailed)
	if entry.Level != zapcore.ErrorLevel {
		t.Fatalf("level = %v, want error below the redelivery threshold", entry.Level)
	}
	if got := logtest.Uint64Field(t, entry, "num_delivered"); got != 1 {
		t.Fatalf("num_delivered = %d, want 1", got)
	}
	if msg.naks != 1 {
		t.Fatalf("naks = %d, want one", msg.naks)
	}
}

func TestHandleMessageRespectsMaxDeliverLogOption(t *testing.T) {
	log, logs := logtest.New()
	// A raised threshold keeps early redeliveries at error level.
	s := newTestSubscriber(log, 5)
	msg := deliveredMsg(testPayload, "req-44", 3)

	s.handleMessage(context.Background(), msg,
		func(context.Context, testEvent) error { return errors.New("handler failed") })

	entry := logtest.Entry(t, logs, messaging.EventConsumeFailed)
	if entry.Level != zapcore.ErrorLevel {
		t.Fatalf("level = %v, want error with a threshold of 5", entry.Level)
	}
}

func TestNewSubscriberOptions(t *testing.T) {
	if got := newSubscriberOptions(nil).maxDeliverLog; got != messaging.DefaultMaxDeliverLog {
		t.Fatalf("default maxDeliverLog = %d, want %d", got, messaging.DefaultMaxDeliverLog)
	}
	if got := newSubscriberOptions([]SubscriberOption{WithMaxDeliverLog(5)}).maxDeliverLog; got != 5 {
		t.Fatalf("maxDeliverLog = %d, want 5", got)
	}
	// Zero keeps the default: it would mark every failure as a retrying one.
	if got := newSubscriberOptions([]SubscriberOption{WithMaxDeliverLog(0)}).maxDeliverLog; got != messaging.DefaultMaxDeliverLog {
		t.Fatalf("zero maxDeliverLog = %d, want the default %d", got, messaging.DefaultMaxDeliverLog)
	}
}

func TestHandleMessageDecodeFailureNaks(t *testing.T) {
	log, logs := logtest.New()
	s := newTestSubscriber(log, 0)
	msg := deliveredMsg([]byte(`{"kit_id":7}`), "req-45", 1)

	called := false
	s.handleMessage(context.Background(), msg,
		func(context.Context, testEvent) error {
			called = true
			return nil
		})
	if called {
		t.Fatal("handler ran on an undecodable payload")
	}
	if msg.naks != 1 || msg.acks != 0 {
		t.Fatalf("acks = %d, naks = %d, want one nak", msg.acks, msg.naks)
	}

	entry := logtest.Entry(t, logs, messaging.EventDecodeFailed)
	if entry.Level != zapcore.ErrorLevel {
		t.Fatalf("level = %v, want error", entry.Level)
	}
	if got := logtest.StringField(t, entry, "stream"); got != testStream {
		t.Fatalf("stream = %q, want %q", got, testStream)
	}
	if got := logtest.StringField(t, entry, "consumer"); got != testConsumer {
		t.Fatalf("consumer = %q, want %q", got, testConsumer)
	}
	if got := logtest.Uint64Field(t, entry, "num_delivered"); got != 1 {
		t.Fatalf("num_delivered = %d, want 1", got)
	}
	if logtest.ErrorField(t, entry) == nil {
		t.Fatal("record has no error")
	}

	logtest.NoEntry(t, logs, messaging.EventConsumeOK)
	logtest.NoPayloadFields(t, logs)
}

func TestHandleMessageAckFailureIsRecorded(t *testing.T) {
	log, logs := logtest.New()
	s := newTestSubscriber(log, 0)
	ackErr := errors.New("ack timed out")
	msg := deliveredMsg(testPayload, "req-46", 1)
	msg.ackErr = ackErr

	s.handleMessage(context.Background(), msg,
		func(context.Context, testEvent) error { return nil })

	entry := logtest.Entry(t, logs, messaging.EventAckFailed)
	if entry.Level != zapcore.ErrorLevel {
		t.Fatalf("level = %v, want error", entry.Level)
	}
	if got := logtest.ErrorField(t, entry); !errors.Is(got, ackErr) {
		t.Fatalf("error = %v, want %v", got, ackErr)
	}
	if got := logtest.StringField(t, entry, "subject"); got != testSubject {
		t.Fatalf("subject = %q, want %q", got, testSubject)
	}
	// The handler itself succeeded, so its own record stays.
	logtest.Entry(t, logs, messaging.EventConsumeOK)
	logtest.NoPayloadFields(t, logs)
}

func TestHandleMessageNakFailureIsRecorded(t *testing.T) {
	log, logs := logtest.New()
	s := newTestSubscriber(log, 0)
	nakErr := errors.New("nak lost")
	msg := deliveredMsg(testPayload, "req-47", 3)
	msg.nakErr = nakErr

	s.handleMessage(context.Background(), msg,
		func(context.Context, testEvent) error { return errors.New("handler failed") })

	// The handler failure is still recorded, and the lost nak is a record of
	// its own: it decides whether the message comes back after the ack wait.
	entry := logtest.Entry(t, logs, messaging.EventNakFailed)
	if entry.Level != zapcore.ErrorLevel {
		t.Fatalf("level = %v, want error", entry.Level)
	}
	if got := logtest.ErrorField(t, entry); !errors.Is(got, nakErr) {
		t.Fatalf("error = %v, want %v", got, nakErr)
	}
	logtest.Entry(t, logs, messaging.EventConsumeFailed)
	logtest.NoPayloadFields(t, logs)
}

func TestHandleMessageWithoutMetadata(t *testing.T) {
	log, logs := logtest.New()
	s := newTestSubscriber(log, 0)
	msg := deliveredMsg(testPayload, "req-48", 0)
	msg.meta = nil
	msg.metaErr = errors.New("no jetstream metadata")

	s.handleMessage(context.Background(), msg,
		func(context.Context, testEvent) error { return errors.New("handler failed") })

	// The loss is recorded instead of ignored...
	logtest.Entry(t, logs, messaging.EventConsumeMetadataFailed)
	// ...and an unknown delivery count cannot downgrade the failure.
	entry := logtest.Entry(t, logs, messaging.EventConsumeFailed)
	if entry.Level != zapcore.ErrorLevel {
		t.Fatalf("level = %v, want error with no delivery count", entry.Level)
	}
	if _, has := logtest.Field(entry, "num_delivered"); has {
		t.Fatalf("record carries num_delivered it cannot know: %+v", entry.Context)
	}
	if got := logtest.StringField(t, entry, "request_id"); got != "req-48" {
		t.Fatalf("request_id = %q, want %q", got, "req-48")
	}
	logtest.NoPayloadFields(t, logs)
}

func TestHandleMessageWithoutLogger(t *testing.T) {
	s := newTestSubscriber(nil, 0)
	msg := deliveredMsg(testPayload, "req-49", 3)

	called := false
	s.handleMessage(context.Background(), msg,
		func(context.Context, testEvent) error {
			called = true
			return errors.New("boom")
		})
	if !called {
		t.Fatal("handler did not run without a logger")
	}
	if msg.naks != 1 {
		t.Fatalf("naks = %d, want one", msg.naks)
	}
}
