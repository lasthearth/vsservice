package mjetstream

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/lasthearth/vsservice/internal/pkg/messaging/logtest"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap/zapcore"
)

func TestPublishAttachesCorrelationHeader(t *testing.T) {
	js := &fakeJS{}
	log, logs := logtest.New()
	p := &Publisher[testEvent]{js: js, stream: testStream, subject: testSubject, log: messaging.Tagged(log)}

	event := testEvent{KitID: "k1", PlayerID: "p1"}
	if err := p.Publish(logtest.CtxWithRequestID("req-31"), event); err != nil {
		t.Fatalf("Publish returned %v, want nil", err)
	}

	if len(js.published) != 1 {
		t.Fatalf("published %d messages, want 1", len(js.published))
	}
	msg := js.published[0]
	if got := msg.Header.Get(messaging.HeaderRequestID); got != "req-31" {
		t.Fatalf("%s = %q, want %q", messaging.HeaderRequestID, got, "req-31")
	}
	// The payload must stay exactly the marshalled event: no correlation field
	// is added to the JSON the external consumers parse.
	want, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	if string(msg.Data) != string(want) {
		t.Fatalf("payload = %s, want %s", msg.Data, want)
	}
	if msg.Subject != testSubject {
		t.Fatalf("subject = %q, want %q", msg.Subject, testSubject)
	}

	entry := logtest.Entry(t, logs, messaging.EventPublishOK)
	if entry.Level != zapcore.DebugLevel {
		t.Fatalf("publish.ok level = %v, want debug", entry.Level)
	}
	if got := logtest.StringField(t, entry, "stream"); got != testStream {
		t.Fatalf("stream = %q, want %q", got, testStream)
	}
	if got := logtest.StringField(t, entry, "subject"); got != testSubject {
		t.Fatalf("subject = %q, want %q", got, testSubject)
	}
	if got := logtest.IntField(t, entry, "payload_bytes"); got != len(want) {
		t.Fatalf("payload_bytes = %d, want %d", got, len(want))
	}
	if got := logtest.Uint64Field(t, entry, "stream_seq"); got != 1 {
		t.Fatalf("stream_seq = %d, want 1", got)
	}
	if got := logtest.StringField(t, entry, "component"); got != messaging.Component {
		t.Fatalf("component = %q, want %q", got, messaging.Component)
	}
	if got := logtest.StringField(t, entry, "request_id"); got != "req-31" {
		t.Fatalf("request_id = %q, want %q", got, "req-31")
	}

	logtest.NoEntry(t, logs, messaging.EventPublishFailed)
	logtest.NoPayloadFields(t, logs)
}

func TestPublishWithoutCorrelationSendsNoHeader(t *testing.T) {
	js := &fakeJS{}
	log, logs := logtest.New()
	p := &Publisher[testEvent]{js: js, stream: testStream, subject: testSubject, log: messaging.Tagged(log)}

	if err := p.Publish(context.Background(), testEvent{KitID: "k1"}); err != nil {
		t.Fatalf("Publish returned %v, want nil", err)
	}

	if h := js.published[0].Header; h != nil {
		t.Fatalf("header = %v, want nil instead of an invented id", h)
	}
	entry := logtest.Entry(t, logs, messaging.EventPublishOK)
	if _, has := logtest.Field(entry, "request_id"); has {
		t.Fatalf("record carries a request_id it cannot know: %+v", entry.Context)
	}
}

func TestPublishWithoutHeaderSupportStillPublishes(t *testing.T) {
	// A zero-value connection reports no header support, which is what an old
	// server does: the publish must degrade, not fail.
	js := &fakeJS{conn: &nats.Conn{}}
	log, logs := logtest.New()
	p := &Publisher[testEvent]{js: js, stream: testStream, subject: testSubject, log: messaging.Tagged(log)}

	if err := p.Publish(logtest.CtxWithRequestID("req-32"), testEvent{KitID: "k1"}); err != nil {
		t.Fatalf("Publish returned %v, want nil", err)
	}
	if len(js.published) != 1 {
		t.Fatalf("published %d messages, want 1", len(js.published))
	}
	if h := js.published[0].Header; h != nil {
		t.Fatalf("header = %v, want nil on a connection without header support", h)
	}
	logtest.Entry(t, logs, messaging.EventPublishOK)
}

func TestPublishFailureIsRecorded(t *testing.T) {
	pubErr := errors.New("no stream response")
	js := &fakeJS{pubErr: pubErr}
	log, logs := logtest.New()
	p := &Publisher[testEvent]{js: js, stream: testStream, subject: testSubject, log: messaging.Tagged(log)}

	err := p.Publish(logtest.CtxWithRequestID("req-33"), testEvent{KitID: "k1"})
	if !errors.Is(err, pubErr) {
		t.Fatalf("Publish returned %v, want %v", err, pubErr)
	}

	entry := logtest.Entry(t, logs, messaging.EventPublishFailed)
	if entry.Level != zapcore.ErrorLevel {
		t.Fatalf("level = %v, want error", entry.Level)
	}
	if got := logtest.StringField(t, entry, "stream"); got != testStream {
		t.Fatalf("stream = %q, want %q", got, testStream)
	}
	if got := logtest.StringField(t, entry, "subject"); got != testSubject {
		t.Fatalf("subject = %q, want %q", got, testSubject)
	}
	if got := logtest.IntField(t, entry, "payload_bytes"); got != len(js.published[0].Data) {
		t.Fatalf("payload_bytes = %d, want %d", got, len(js.published[0].Data))
	}
	if got := logtest.ErrorField(t, entry); !errors.Is(got, pubErr) {
		t.Fatalf("error = %v, want %v", got, pubErr)
	}

	logtest.NoEntry(t, logs, messaging.EventPublishOK)
	logtest.NoPayloadFields(t, logs)
}

func TestPublishEncodeFailureIsRecorded(t *testing.T) {
	js := &fakeJS{}
	log, logs := logtest.New()
	p := &Publisher[undecodableEvent]{js: js, stream: testStream, subject: testSubject, log: messaging.Tagged(log)}

	err := p.Publish(context.Background(), undecodableEvent{Ch: make(chan int)})
	if err == nil {
		t.Fatal("Publish succeeded, want the encode error")
	}
	if len(js.published) != 0 {
		t.Fatalf("published %d messages, want none", len(js.published))
	}

	entry := logtest.Entry(t, logs, messaging.EventPublishEncodeFailed)
	if entry.Level != zapcore.ErrorLevel {
		t.Fatalf("level = %v, want error", entry.Level)
	}
	if got := logtest.StringField(t, entry, "stream"); got != testStream {
		t.Fatalf("stream = %q, want %q", got, testStream)
	}
	if logtest.ErrorField(t, entry) == nil {
		t.Fatal("record has no error")
	}

	logtest.NoEntry(t, logs, messaging.EventPublishOK)
	logtest.NoPayloadFields(t, logs)
}

func TestPublishWithoutLogger(t *testing.T) {
	// Call sites may build a publisher without a logger: the guard must hold
	// and the publish must still run.
	js := &fakeJS{}
	p := &Publisher[testEvent]{js: js, stream: testStream, subject: testSubject}

	if err := p.Publish(logtest.CtxWithRequestID("req-34"), testEvent{KitID: "k1"}); err != nil {
		t.Fatalf("Publish returned %v, want nil", err)
	}
	if len(js.published) != 1 {
		t.Fatalf("published %d messages, want 1", len(js.published))
	}
}

// TestMsgIDFromBytesIsStable guards the dedupe contract: the id stays the
// sha256 of the exact bytes going on the wire.
func TestMsgIDFromBytesIsStable(t *testing.T) {
	sum := sha256.Sum256(testPayload)
	want := hex.EncodeToString(sum[:])

	if got := msgIDFromBytes(testPayload); got != want {
		t.Fatalf("msgIDFromBytes = %q, want %q", got, want)
	}
	if got := msgIDFromBytes(testPayload); got != want {
		t.Fatalf("msgIDFromBytes is not stable: %q, want %q", got, want)
	}
	if got := msgIDFromBytes([]byte(`{"kit_id":"k2"}`)); got == want {
		t.Fatal("msgIDFromBytes returned the same id for another payload")
	}
}

func TestHeadersSupported(t *testing.T) {
	if got := headersSupported(&fakeJS{conn: nil}); !got {
		t.Fatal("headersSupported with no connection = false, want true so fakes stay exercised")
	}
	if got := headersSupported(&fakeJS{conn: &nats.Conn{}}); got {
		t.Fatal("headersSupported with a connection that reports no headers = true, want false")
	}
}
