package mnats

import (
	"context"
	"errors"
	"testing"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/lasthearth/vsservice/internal/pkg/messaging/logtest"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap/zapcore"
)

// newTestSubscriber builds a subscriber without a connection: handleMsg is the
// traced part and it never touches the server.
func newTestSubscriber(log logger.Logger) *eventSubscriber[testEvent] {
	return &eventSubscriber[testEvent]{
		natsBus: &natsBus{
			subject: testSubject,
			encoder: JsonEncoder,
			// Tagged is what WithLogger does, so the records carry the component.
			log: messaging.Tagged(log),
		},
	}
}

func testMsg(payload []byte, requestID string) *nats.Msg {
	m := &nats.Msg{Subject: testSubject, Data: payload}
	m.Header = messaging.OutgoingHeader(logtest.CtxWithRequestID(requestID), true)
	return m
}

func TestHandleMsgInjectsRequestIDAndRecordsSuccess(t *testing.T) {
	log, logs := logtest.New()
	s := newTestSubscriber(log)

	var gotRID string
	var gotEvent testEvent
	s.handleMsg(context.Background(), testMsg(testPayload, "req-5"),
		func(ctx context.Context, event testEvent) error {
			// The id must reach the handler ctx, so its own records and any
			// message it publishes keep the same correlation.
			gotRID = logger.RequestID(ctx)
			gotEvent = event
			return nil
		})

	if gotRID != "req-5" {
		t.Fatalf("handler ctx request_id = %q, want %q", gotRID, "req-5")
	}
	if gotEvent.PlayerID != "p1" || gotEvent.Coins != 3 {
		t.Fatalf("handler got %+v, want the decoded payload", gotEvent)
	}

	start := logtest.Entry(t, logs, messaging.EventConsumeStart)
	if start.Level != zapcore.DebugLevel {
		t.Fatalf("consume.start level = %v, want debug", start.Level)
	}
	if got := logtest.StringField(t, start, "subject"); got != testSubject {
		t.Fatalf("subject = %q, want %q", got, testSubject)
	}
	if got := logtest.IntField(t, start, "payload_bytes"); got != len(testPayload) {
		t.Fatalf("payload_bytes = %d, want %d", got, len(testPayload))
	}
	if got := logtest.StringField(t, start, "component"); got != messaging.Component {
		t.Fatalf("component = %q, want %q", got, messaging.Component)
	}
	if got := logtest.StringField(t, start, "request_id"); got != "req-5" {
		t.Fatalf("request_id = %q, want %q", got, "req-5")
	}

	ok := logtest.Entry(t, logs, messaging.EventConsumeOK)
	if ok.Level != zapcore.DebugLevel {
		t.Fatalf("consume.ok level = %v, want debug", ok.Level)
	}
	if got := logtest.StringField(t, ok, "subject"); got != testSubject {
		t.Fatalf("subject = %q, want %q", got, testSubject)
	}

	logtest.NoEntry(t, logs, messaging.EventConsumeFailed)
	logtest.NoEntry(t, logs, messaging.EventDecodeFailed)
	logtest.NoPayloadFields(t, logs)
}

func TestHandleMsgWithoutHeaderKeepsWorking(t *testing.T) {
	log, logs := logtest.New()
	s := newTestSubscriber(log)

	var gotRID = "unset"
	// A publisher on an old server sends no header: the delivery is still
	// processed and no id is invented.
	s.handleMsg(context.Background(), &nats.Msg{Subject: testSubject, Data: testPayload},
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
}

func TestHandleMsgFallsBackToConfiguredSubject(t *testing.T) {
	log, logs := logtest.New()
	s := newTestSubscriber(log)

	// A replayed or hand-built message may carry no subject; the subscription
	// filter still identifies the traffic.
	s.handleMsg(context.Background(), &nats.Msg{Data: testPayload},
		func(context.Context, testEvent) error { return nil })

	entry := logtest.Entry(t, logs, messaging.EventConsumeStart)
	if got := logtest.StringField(t, entry, "subject"); got != testSubject {
		t.Fatalf("subject = %q, want %q", got, testSubject)
	}
}

func TestHandleMsgDecodeFailureIsRecorded(t *testing.T) {
	log, logs := logtest.New()
	s := newTestSubscriber(log)

	called := false
	s.handleMsg(context.Background(), testMsg([]byte(`{"coins":"not-a-number"}`), "req-6"),
		func(context.Context, testEvent) error {
			called = true
			return nil
		})
	if called {
		t.Fatal("handler ran on an undecodable payload")
	}

	entry := logtest.Entry(t, logs, messaging.EventDecodeFailed)
	if entry.Level != zapcore.ErrorLevel {
		t.Fatalf("level = %v, want error", entry.Level)
	}
	if got := logtest.StringField(t, entry, "subject"); got != testSubject {
		t.Fatalf("subject = %q, want %q", got, testSubject)
	}
	if got := logtest.StringField(t, entry, "request_id"); got != "req-6" {
		t.Fatalf("request_id = %q, want %q", got, "req-6")
	}
	if logtest.ErrorField(t, entry) == nil {
		t.Fatal("record has no error")
	}

	logtest.NoEntry(t, logs, messaging.EventConsumeOK)
	logtest.NoPayloadFields(t, logs)
}

func TestHandleMsgHandlerFailureIsRecorded(t *testing.T) {
	log, logs := logtest.New()
	s := newTestSubscriber(log)

	handlerErr := errors.New("kit grant failed")
	s.handleMsg(context.Background(), testMsg(testPayload, "req-7"),
		func(context.Context, testEvent) error { return handlerErr })

	entry := logtest.Entry(t, logs, messaging.EventConsumeFailed)
	// Core NATS redelivers nothing, so there is no delivery count to downgrade
	// the record: it stays an error.
	if entry.Level != zapcore.ErrorLevel {
		t.Fatalf("level = %v, want error", entry.Level)
	}
	if got := logtest.ErrorField(t, entry); !errors.Is(got, handlerErr) {
		t.Fatalf("error = %v, want %v", got, handlerErr)
	}
	if got := logtest.StringField(t, entry, "request_id"); got != "req-7" {
		t.Fatalf("request_id = %q, want %q", got, "req-7")
	}
	if got := logtest.IntField(t, entry, "payload_bytes"); got != len(testPayload) {
		t.Fatalf("payload_bytes = %d, want %d", got, len(testPayload))
	}

	logtest.NoEntry(t, logs, messaging.EventConsumeOK)
	logtest.NoPayloadFields(t, logs)
}

func TestHandleMsgWithoutLogger(t *testing.T) {
	s := newTestSubscriber(nil)

	called := false
	s.handleMsg(context.Background(), testMsg(testPayload, "req-8"),
		func(context.Context, testEvent) error {
			called = true
			return errors.New("boom")
		})
	if !called {
		t.Fatal("handler did not run without a logger")
	}
}
