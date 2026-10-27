package messaging

import (
	"context"
	"testing"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/messaging/logtest"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap/zapcore"
)

func TestOutgoingHeader(t *testing.T) {
	ctxWithID := logtest.CtxWithRequestID("req-42")

	t.Run("carries the correlation id when headers are supported", func(t *testing.T) {
		h := OutgoingHeader(ctxWithID, true)
		if h == nil {
			t.Fatal("header is nil, want the correlation id")
		}
		if got := h.Get(HeaderRequestID); got != "req-42" {
			t.Fatalf("%s = %q, want %q", HeaderRequestID, got, "req-42")
		}
		// nats.Header is case-sensitive, so the key is stored in its canonical
		// spelling and a raw lowercase lookup misses it; RequestIDFromHeader is
		// what accepts any spelling.
		if got := h.Get("x-request-id"); got != "" {
			t.Fatalf("lowercase lookup = %q, want empty for a case-sensitive map", got)
		}
		if got := RequestIDFromHeader(h); got != "req-42" {
			t.Fatalf("RequestIDFromHeader = %q, want %q", got, "req-42")
		}
		if got := RequestIDFromHeader(nats.Header{"x-request-id": {"req-43"}}); got != "req-43" {
			t.Fatalf("lowercase header = %q, want %q", got, "req-43")
		}
		if len(h) != 1 {
			t.Fatalf("header has %d entries, want 1: %v", len(h), h)
		}
	})

	t.Run("no header when the server does not support headers", func(t *testing.T) {
		if h := OutgoingHeader(ctxWithID, false); h != nil {
			t.Fatalf("header = %v, want nil so the publish still succeeds", h)
		}
	})

	t.Run("no header when ctx carries no id", func(t *testing.T) {
		if h := OutgoingHeader(context.Background(), true); h != nil {
			t.Fatalf("header = %v, want nil instead of an invented id", h)
		}
	})
}

func TestRequestIDFromHeader(t *testing.T) {
	h := OutgoingHeader(logtest.CtxWithRequestID("req-7"), true)

	if got := RequestIDFromHeader(h); got != "req-7" {
		t.Fatalf("RequestIDFromHeader = %q, want %q", got, "req-7")
	}
	if got := RequestIDFromHeader(nil); got != "" {
		t.Fatalf("RequestIDFromHeader(nil) = %q, want empty", got)
	}
	if got := RequestIDFromHeader(map[string][]string{"Other": {"x"}}); got != "" {
		t.Fatalf("RequestIDFromHeader without the id = %q, want empty", got)
	}
}

func TestCtxWithRequestID(t *testing.T) {
	ctx := CtxWithRequestID(context.Background(), "req-9")
	if got := logger.RequestID(ctx); got != "req-9" {
		t.Fatalf("logger.RequestID = %q, want %q", got, "req-9")
	}

	// An empty id must not clear or invent anything.
	base := CtxWithRequestID(context.Background(), "keep-me")
	if got := CtxWithRequestID(base, ""); got != base {
		t.Fatal("empty id returned a new ctx, want the original")
	}
	if got := logger.RequestID(CtxWithRequestID(base, "")); got != "keep-me" {
		t.Fatalf("logger.RequestID = %q, want %q", got, "keep-me")
	}

	// The id must reach a record built through logger.From, which is what the
	// consume side uses.
	log, logs := logtest.New()
	logger.From(ctx, log).Debug("downstream")
	entry := logtest.Entry(t, logs, "downstream")
	if got := logtest.StringField(t, entry, "request_id"); got != "req-9" {
		t.Fatalf("request_id = %q, want %q", got, "req-9")
	}
}

func TestCtxWithDelivered(t *testing.T) {
	ctx := CtxWithDelivered(context.Background(), 4)

	fields := logging.ExtractFields(ctx)
	var found bool
	for i := 0; i+1 < len(fields); i += 2 {
		key, ok := fields[i].(string)
		if !ok || key != "num_delivered" {
			continue
		}
		if got, ok := fields[i+1].(uint64); !ok || got != 4 {
			t.Fatalf("num_delivered = %v, want uint64(4)", fields[i+1])
		}
		found = true
	}
	if !found {
		t.Fatalf("ctx carries no num_delivered: %v", fields)
	}

	log, logs := logtest.New()
	logger.From(ctx, log).Debug("downstream")
	entry := logtest.Entry(t, logs, "downstream")
	if got := logtest.Uint64Field(t, entry, "num_delivered"); got != 4 {
		t.Fatalf("num_delivered = %d, want 4", got)
	}
}

func TestHandlerFailureLevel(t *testing.T) {
	cases := []struct {
		numDelivered uint64
		maxDeliver   uint64
		want         zapcore.Level
	}{
		// An unknown delivery count cannot downgrade a failure.
		{numDelivered: 0, maxDeliver: DefaultMaxDeliverLog, want: zapcore.ErrorLevel},
		{numDelivered: 1, maxDeliver: DefaultMaxDeliverLog, want: zapcore.ErrorLevel},
		{numDelivered: 2, maxDeliver: DefaultMaxDeliverLog, want: zapcore.ErrorLevel},
		{numDelivered: 3, maxDeliver: DefaultMaxDeliverLog, want: zapcore.WarnLevel},
		{numDelivered: 9, maxDeliver: DefaultMaxDeliverLog, want: zapcore.WarnLevel},
		{numDelivered: 1, maxDeliver: 1, want: zapcore.WarnLevel},
	}

	for _, c := range cases {
		if got := HandlerFailureLevel(c.numDelivered, c.maxDeliver); got != c.want {
			t.Errorf("HandlerFailureLevel(%d, %d) = %v, want %v",
				c.numDelivered, c.maxDeliver, got, c.want)
		}
	}
}

func TestLog(t *testing.T) {
	ctx := logtest.CtxWithRequestID("req-1")

	if got := Log(ctx, nil); got != nil {
		t.Fatalf("Log with a nil fallback = %v, want nil so the guards hold", got)
	}

	log, logs := logtest.New()
	tagged := Tagged(log)
	if tagged == nil {
		t.Fatal("Tagged dropped a non-nil logger")
	}
	if got := Tagged(nil); got != nil {
		t.Fatalf("Tagged(nil) = %v, want nil", got)
	}

	Log(ctx, tagged).Info("record")
	entry := logtest.Entry(t, logs, "record")
	if got := logtest.StringField(t, entry, "component"); got != Component {
		t.Fatalf("component = %q, want %q", got, Component)
	}
	if got := logtest.StringField(t, entry, "request_id"); got != "req-1" {
		t.Fatalf("request_id = %q, want %q", got, "req-1")
	}
	if entry.Level != zapcore.InfoLevel {
		t.Fatalf("level = %v, want info", entry.Level)
	}
}

func TestRecordSubject(t *testing.T) {
	if got := RecordSubject("delivered.subject", "filter.subject"); got != "delivered.subject" {
		t.Fatalf("RecordSubject = %q, want the delivered subject", got)
	}
	if got := RecordSubject("", "filter.subject"); got != "filter.subject" {
		t.Fatalf("RecordSubject = %q, want the configured filter", got)
	}
}
