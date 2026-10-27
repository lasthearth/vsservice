package mnats

import (
	"context"
	"testing"

	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/lasthearth/vsservice/internal/pkg/messaging/logtest"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type testEvent struct {
	PlayerID string `json:"player_id"`
	Coins    int    `json:"coins"`
}

const testSubject = "player.join"

// testPayload is the exact wire format the publishers must not change: the
// correlation id travels in a header instead.
var testPayload = []byte(`{"player_id":"p1","coins":3}`)

func TestPublishMsgCarriesCorrelationHeader(t *testing.T) {
	ctx := logtest.CtxWithRequestID("req-11")

	msg := publishMsg(ctx, true, testSubject, testPayload)

	if got := msg.Header.Get(messaging.HeaderRequestID); got != "req-11" {
		t.Fatalf("%s = %q, want %q", messaging.HeaderRequestID, got, "req-11")
	}
	if msg.Subject != testSubject {
		t.Fatalf("subject = %q, want %q", msg.Subject, testSubject)
	}
	if string(msg.Data) != string(testPayload) {
		t.Fatalf("payload = %s, want it byte for byte unchanged", msg.Data)
	}
	if msg.Reply != "" {
		t.Fatalf("reply = %q, want empty for a plain publish", msg.Reply)
	}
}

func TestPublishMsgWithoutHeaderSupport(t *testing.T) {
	msg := publishMsg(logtest.CtxWithRequestID("req-11"), false, testSubject, testPayload)

	if msg.Header != nil {
		t.Fatalf("header = %v, want nil so an old server still accepts the message", msg.Header)
	}
	if string(msg.Data) != string(testPayload) {
		t.Fatalf("payload = %s, want it unchanged", msg.Data)
	}
}

func TestPublishMsgWithoutCorrelation(t *testing.T) {
	msg := publishMsg(context.Background(), true, testSubject, testPayload)

	if msg.Header != nil {
		t.Fatalf("header = %v, want nil instead of an invented id", msg.Header)
	}
}

// newTestPublisher builds a publisher over a zero-value connection. The
// connection is never dialled: the tests below only reach the encode step.
func newTestPublisher(t *testing.T, encoder QueueEncoder) (messaging.Publisher[testEvent], *observer.ObservedLogs) {
	t.Helper()
	log, logs := logtest.New()
	p := NewEventPublisher[testEvent](&nats.Conn{}, testSubject,
		WithEncoder(encoder), WithLogger(log))
	return p, logs
}

func TestPublishEncodeFailureIsRecorded(t *testing.T) {
	// An unknown encoder fails before the connection is touched.
	p, logs := newTestPublisher(t, QueueEncoder("bogus"))

	err := p.Publish(context.Background(), testEvent{PlayerID: "p1", Coins: 3})
	if err == nil {
		t.Fatal("Publish succeeded, want the encode error")
	}

	entry := logtest.Entry(t, logs, messaging.EventPublishEncodeFailed)
	if entry.Level != zapcore.ErrorLevel {
		t.Fatalf("level = %v, want error", entry.Level)
	}
	if got := logtest.StringField(t, entry, "subject"); got != testSubject {
		t.Fatalf("subject = %q, want %q", got, testSubject)
	}
	if got := logtest.StringField(t, entry, "component"); got != messaging.Component {
		t.Fatalf("component = %q, want %q", got, messaging.Component)
	}
	if logtest.ErrorField(t, entry) == nil {
		t.Fatal("record has no error")
	}

	logtest.NoEntry(t, logs, messaging.EventPublishOK)
	logtest.NoEntry(t, logs, messaging.EventPublishFailed)
	logtest.NoPayloadFields(t, logs)
}

func TestPublishEncodeFailureWithoutLogger(t *testing.T) {
	// Call sites may build a bus without a logger: the guard must hold and the
	// error must still reach the caller.
	p := NewEventPublisher[testEvent](&nats.Conn{}, testSubject, WithEncoder(QueueEncoder("bogus")))

	if err := p.Publish(context.Background(), testEvent{}); err == nil {
		t.Fatal("Publish succeeded, want the encode error")
	}
}
