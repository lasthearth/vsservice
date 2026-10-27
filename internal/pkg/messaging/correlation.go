package messaging

import (
	"context"
	"strings"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap/zapcore"
)

// HeaderRequestID carries the correlation id of the request that caused a
// message. It travels in the NATS message header, never in the payload: the
// game server and the mail backend consume these subjects today and parse the
// payload as it is, while a header they do not read keeps the wire format
// intact and still makes the traffic replayable.
const HeaderRequestID = "X-Request-Id"

// Component is the log component every messaging record carries.
const Component = "nats"

// DefaultMaxDeliverLog mirrors the NATS_MAX_DELIVER_LOG default of
// config.Config. The domain fx constructors build subscribers with fixed
// arguments, so they cannot pass the config value yet; this fallback keeps the
// same rule until they adopt WithMaxDeliverLog.
const DefaultMaxDeliverLog uint64 = 3

// Record names of the messaging traffic log. Every record carries the
// component plus enough identity to trace one hop: subject, payload size,
// stream, consumer and delivery count. The payload itself is never logged.
const (
	EventPublishOK             = "nats.publish.ok"
	EventPublishFailed         = "nats.publish.failed"
	EventPublishEncodeFailed   = "nats.publish.encode_failed"
	EventConsumeStart          = "nats.consume.start"
	EventConsumeOK             = "nats.consume.ok"
	EventConsumeFailed         = "nats.consume.failed"
	EventConsumeMetadataFailed = "nats.consume.metadata_failed"
	EventDecodeFailed          = "nats.decode.failed"
	EventAckFailed             = "nats.ack.failed"
	EventNakFailed             = "nats.nak.failed"
)

// OutgoingHeader returns the headers an outgoing message must carry. It returns
// nil when ctx carries no correlation id, because inventing one would break the
// join with the originating request's records. It also returns nil when
// headersOK is false: a server that predates headers rejects the whole message,
// and losing the correlation id costs less than losing the event.
func OutgoingHeader(ctx context.Context, headersOK bool) nats.Header {
	if !headersOK {
		return nil
	}

	rid := logger.RequestID(ctx)
	if rid == "" {
		return nil
	}

	// nats.Header lookups are case-sensitive, so the key is stored in its
	// canonical HTTP spelling and consumers read it through
	// RequestIDFromHeader, which accepts any spelling.
	h := nats.Header{}
	h.Set(HeaderRequestID, rid)
	return h
}

// RequestIDFromHeader reads the correlation id a publisher attached to an
// incoming message. It returns "" when the header is absent, so the caller
// skips injection instead of clearing an id ctx already carries. The lookup
// accepts any key spelling: nats.Header is case-sensitive, and a publisher in
// another language may send the lowercase form.
func RequestIDFromHeader(h nats.Header) string {
	if h == nil {
		return ""
	}
	if rid := h.Get(HeaderRequestID); rid != "" {
		return rid
	}
	for key, values := range h {
		if !strings.EqualFold(key, HeaderRequestID) || len(values) == 0 {
			continue
		}
		return values[0]
	}
	return ""
}

// CtxWithRequestID returns ctx carrying requestID under the same key the gRPC
// middleware uses, so logger.From adds it to every downstream record and any
// message the handler publishes keeps the same id. An empty requestID returns
// ctx unchanged.
func CtxWithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	return logging.InjectFields(ctx, logging.Fields{"request_id", requestID})
}

// CtxWithDelivered adds the delivery attempt to ctx, so a handler's own records
// show whether it runs on a first delivery or on a redelivery.
func CtxWithDelivered(ctx context.Context, numDelivered uint64) context.Context {
	return logging.InjectFields(ctx, logging.Fields{"num_delivered", numDelivered})
}

// HandlerFailureLevel is the level of a handler-failure record. At or above
// maxDeliverLog the message is a known retrying one, so the record is Warn and
// the first failures stay Error. A zero numDelivered means the delivery count
// is unknown, and an unknown count cannot downgrade a failure.
func HandlerFailureLevel(numDelivered, maxDeliverLog uint64) zapcore.Level {
	if numDelivered >= maxDeliverLog {
		return zapcore.WarnLevel
	}
	return zapcore.ErrorLevel
}

// Log returns the record logger of a messaging call: the bus logger enriched
// with the correlation fields ctx carries, which is what logger.From does. It
// keeps nil when fallback is nil, because some call sites build a bus without a
// logger and their nil guards must keep working.
func Log(ctx context.Context, fallback logger.Logger) logger.Logger {
	if fallback == nil {
		return nil
	}
	return logger.From(ctx, fallback)
}

// Tagged marks a bus logger as the source of messaging records. Tagging once at
// construction keeps the component field out of every publish and consume call.
func Tagged(l logger.Logger) logger.Logger {
	if l == nil {
		return nil
	}
	return l.WithComponent(Component)
}

// RecordSubject picks the subject of a record: the one the delivered message
// carries, which stays correct for a wildcard subscription, or the configured
// filter when the message has none, as with a hand-built message in a test.
func RecordSubject(msgSubject, filter string) string {
	if msgSubject != "" {
		return msgSubject
	}
	return filter
}
