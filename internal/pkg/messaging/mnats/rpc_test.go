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

const (
	rpcSubject = "player.try-join"
	rpcInbox   = "_INBOX.test"
)

type testReq struct {
	PlayerID string `json:"player_id"`
}

type testResp struct {
	Allowed bool `json:"allowed"`
}

// newTestResponder builds a responder without a connection: the traced paths
// below return before the reply is published.
func newTestResponder(log logger.Logger) *rpcResponder[testReq, testResp] {
	return &rpcResponder[testReq, testResp]{
		natsBus: &natsBus{
			subject: rpcSubject,
			encoder: JsonEncoder,
			log:     messaging.Tagged(log),
		},
		queueGroup: messaging.DefaultWorkerGroup,
	}
}

func rpcMsg(payload []byte, requestID string) *nats.Msg {
	m := &nats.Msg{Subject: rpcSubject, Reply: rpcInbox, Data: payload}
	m.Header = messaging.OutgoingHeader(logtest.CtxWithRequestID(requestID), true)
	return m
}

func TestRpcHandleMsgInjectsRequestID(t *testing.T) {
	log, logs := logtest.New()
	r := newTestResponder(log)

	var gotRID string
	handlerErr := errors.New("player not found")
	r.handleMsg(context.Background(), rpcMsg([]byte(`{"player_id":"p1"}`), "req-21"),
		func(ctx context.Context, req testReq) (testResp, error) {
			// The id must reach the handler ctx, so its own records and any
			// message it publishes keep the same correlation.
			gotRID = logger.RequestID(ctx)
			if req.PlayerID != "p1" {
				t.Errorf("request = %+v, want player_id p1", req)
			}
			return testResp{}, handlerErr
		})

	if gotRID != "req-21" {
		t.Fatalf("handler ctx request_id = %q, want %q", gotRID, "req-21")
	}

	start := logtest.Entry(t, logs, messaging.EventConsumeStart)
	if got := logtest.StringField(t, start, "subject"); got != rpcSubject {
		t.Fatalf("subject = %q, want %q", got, rpcSubject)
	}
	if got := logtest.StringField(t, start, "component"); got != messaging.Component {
		t.Fatalf("component = %q, want %q", got, messaging.Component)
	}
	if got := logtest.IntField(t, start, "payload_bytes"); got != len(`{"player_id":"p1"}`) {
		t.Fatalf("payload_bytes = %d, want %d", got, len(`{"player_id":"p1"}`))
	}

	failed := logtest.Entry(t, logs, messaging.EventConsumeFailed)
	if failed.Level != zapcore.ErrorLevel {
		t.Fatalf("level = %v, want error", failed.Level)
	}
	if got := logtest.StringField(t, failed, "request_id"); got != "req-21" {
		t.Fatalf("request_id = %q, want %q", got, "req-21")
	}
	if got := logtest.ErrorField(t, failed); !errors.Is(got, handlerErr) {
		t.Fatalf("error = %v, want %v", got, handlerErr)
	}

	logtest.NoEntry(t, logs, messaging.EventConsumeOK)
	logtest.NoPayloadFields(t, logs)
}

func TestRpcHandleMsgDecodeFailureIsRecorded(t *testing.T) {
	log, logs := logtest.New()
	r := newTestResponder(log)

	called := false
	r.handleMsg(context.Background(), rpcMsg([]byte(`{"player_id":7}`), "req-22"),
		func(context.Context, testReq) (testResp, error) {
			called = true
			return testResp{}, nil
		})
	if called {
		t.Fatal("handler ran on an undecodable request")
	}

	entry := logtest.Entry(t, logs, messaging.EventDecodeFailed)
	if entry.Level != zapcore.ErrorLevel {
		t.Fatalf("level = %v, want error", entry.Level)
	}
	if got := logtest.StringField(t, entry, "subject"); got != rpcSubject {
		t.Fatalf("subject = %q, want %q", got, rpcSubject)
	}
	if got := logtest.StringField(t, entry, "request_id"); got != "req-22" {
		t.Fatalf("request_id = %q, want %q", got, "req-22")
	}
	if logtest.ErrorField(t, entry) == nil {
		t.Fatal("record has no error")
	}

	logtest.NoEntry(t, logs, messaging.EventConsumeOK)
	logtest.NoPayloadFields(t, logs)
}

func TestRpcHandleMsgTracesReplyPublish(t *testing.T) {
	log, logs := logtest.New()
	r := newTestResponder(log)

	r.handleMsg(context.Background(), rpcMsg([]byte(`{"player_id":"p1"}`), "req-23"),
		func(context.Context, testReq) (testResp, error) {
			return testResp{Allowed: true}, nil
		})

	// The handler succeeded, so its record comes first and carries the id.
	ok := logtest.Entry(t, logs, messaging.EventConsumeOK)
	if got := logtest.StringField(t, ok, "request_id"); got != "req-23" {
		t.Fatalf("request_id = %q, want %q", got, "req-23")
	}

	// The responder has no connection, so the reply cannot go out. The failure
	// is traced against the reply inbox instead of being swallowed.
	entry := logtest.Entry(t, logs, messaging.EventPublishFailed)
	if got := logtest.StringField(t, entry, "subject"); got != rpcInbox {
		t.Fatalf("subject = %q, want the reply inbox %q", got, rpcInbox)
	}
	if got := logtest.ErrorField(t, entry); !errors.Is(got, errNoConnection) {
		t.Fatalf("error = %v, want %v", got, errNoConnection)
	}
	logtest.NoPayloadFields(t, logs)
}

func TestRpcHandleMsgWithoutLogger(t *testing.T) {
	r := newTestResponder(nil)

	called := false
	r.handleMsg(context.Background(), rpcMsg([]byte(`{"player_id":"p1"}`), "req-24"),
		func(context.Context, testReq) (testResp, error) {
			called = true
			return testResp{}, errors.New("boom")
		})
	if !called {
		t.Fatal("handler did not run without a logger")
	}
}

func TestRpcRequestMsgCarriesReplyAndCorrelation(t *testing.T) {
	// The requester builds its message the same way a publisher does and then
	// sets the inbox, so both parts must survive.
	msg := publishMsg(logtest.CtxWithRequestID("req-25"), true, rpcSubject, []byte(`{"player_id":"p1"}`))
	msg.Reply = nats.NewInbox()

	if got := msg.Header.Get(messaging.HeaderRequestID); got != "req-25" {
		t.Fatalf("%s = %q, want %q", messaging.HeaderRequestID, got, "req-25")
	}
	if msg.Reply == "" {
		t.Fatal("reply inbox is empty, want one for the responder to answer on")
	}
	if msg.Subject != rpcSubject {
		t.Fatalf("subject = %q, want %q", msg.Subject, rpcSubject)
	}
}
