package mjetstream

import (
	"context"
	"encoding/json"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Subscriber[T any] struct {
	js            jetstream.JetStream
	stream        string
	subject       string
	consName      string
	cons          jetstream.Consumer
	consCtx       jetstream.ConsumeContext
	group         string
	log           logger.Logger
	timeout       time.Duration
	maxDeliverLog uint64
	cancel        context.CancelFunc
}

// subscriberOptions holds the tunables a SubscriberOption sets. It is not
// generic, so one option type serves every Subscriber[T].
type subscriberOptions struct {
	maxDeliverLog uint64
}

// SubscriberOption tunes a subscriber after the positional arguments, which
// keeps NewSubscriber's signature additive: the domain fx constructors call it
// with fixed arguments and must keep compiling.
type SubscriberOption func(*subscriberOptions)

// WithMaxDeliverLog sets the delivery attempt from which a handler failure is
// logged as Warn instead of Error. Pass cfg.NatsMaxDeliverLog to follow the
// NATS_MAX_DELIVER_LOG setting. Zero keeps the default, because a zero
// threshold would mark every failure as a known retrying message.
func WithMaxDeliverLog(n uint64) SubscriberOption {
	return func(o *subscriberOptions) {
		o.maxDeliverLog = n
	}
}

func newSubscriberOptions(opts []SubscriberOption) subscriberOptions {
	o := subscriberOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	if o.maxDeliverLog == 0 {
		o.maxDeliverLog = messaging.DefaultMaxDeliverLog
	}
	return o
}

func NewSubscriber[T any](
	js jetstream.JetStream,
	stream, subject, consumer, group string,
	log logger.Logger,
	opts ...SubscriberOption,
) (messaging.Subscriber[T], error) {
	if err := ensureStream(js, stream, []string{subject}); err != nil {
		return nil, err
	}
	cons, err := ensureConsumer(js, stream, consumer, subject, group)
	if err != nil {
		return nil, err
	}

	o := newSubscriberOptions(opts)

	return &Subscriber[T]{
		js:       js,
		stream:   stream,
		subject:  subject,
		consName: consumer,
		cons:     cons,
		group:    group,
		// Tagged adds the nats component once here instead of on every record.
		// A nil logger stays nil: call sites without one keep their guard.
		log:           messaging.Tagged(log),
		timeout:       30 * time.Second,
		maxDeliverLog: o.maxDeliverLog,
	}, nil
}

func (s *Subscriber[T]) Subscribe(handler func(ctx context.Context, event T) error) error {
	//nolint:gosec // G118: cancel is stored in s.cancel and invoked by Unsubscribe
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	consCtx, err := s.cons.Consume(func(msg jetstream.Msg) {
		s.handleMessage(ctx, msg, handler)
	})
	if err != nil {
		return err
	}

	s.consCtx = consCtx
	return nil
}

// handleMessage decodes one delivery, runs the handler and acks or naks it. It
// is split from Subscribe so the tracing rules stay testable without a live
// JetStream server.
func (s *Subscriber[T]) handleMessage(
	ctx context.Context,
	msg jetstream.Msg,
	handler func(context.Context, T) error,
) {
	subject := messaging.RecordSubject(msg.Subject(), s.subject)
	payload := msg.Data()

	// The publisher put the correlation id of the originating request in the
	// header. Injecting it into ctx keeps the handler's own records, and any
	// message it publishes, under the same id.
	ctx = messaging.CtxWithRequestID(ctx, messaging.RequestIDFromHeader(msg.Headers()))

	// Metadata carries the delivery attempt. Without it the level rule cannot
	// tell a retrying message from a first failure, so the loss is recorded and
	// the delivery is still processed.
	var numDelivered uint64
	var hasMeta bool
	if md, err := msg.Metadata(); err != nil {
		if log := messaging.Log(ctx, s.log); log != nil {
			log.Debug(messaging.EventConsumeMetadataFailed,
				append(s.recordFields(subject, len(payload), 0, false), zap.Error(err))...)
		}
	} else {
		numDelivered = md.NumDelivered
		hasMeta = true
		ctx = messaging.CtxWithDelivered(ctx, numDelivered)
	}

	log := messaging.Log(ctx, s.log)
	if log != nil {
		log.Debug(messaging.EventConsumeStart,
			s.recordFields(subject, len(payload), numDelivered, hasMeta)...)
	}

	var event T
	if err := json.Unmarshal(payload, &event); err != nil {
		if log != nil {
			log.Error(messaging.EventDecodeFailed,
				append(s.recordFields(subject, len(payload), numDelivered, hasMeta), zap.Error(err))...)
		}
		s.nak(msg, log, subject, numDelivered, hasMeta)
		return
	}

	if err := handler(ctx, event); err != nil {
		if log != nil {
			fields := append(s.recordFields(subject, len(payload), numDelivered, hasMeta), zap.Error(err))
			if messaging.HandlerFailureLevel(numDelivered, s.maxDeliverLog) == zapcore.WarnLevel {
				// The server already redelivered this message maxDeliverLog
				// times, so it is a known retrying message, not a new fault.
				log.Warn(messaging.EventConsumeFailed, fields...)
			} else {
				log.Error(messaging.EventConsumeFailed, fields...)
			}
		}
		s.nak(msg, log, subject, numDelivered, hasMeta)
		return
	}

	if log != nil {
		log.Debug(messaging.EventConsumeOK,
			s.recordFields(subject, len(payload), numDelivered, hasMeta)...)
	}

	if err := msg.Ack(); err != nil {
		if log != nil {
			log.Error(messaging.EventAckFailed,
				append(s.recordFields(subject, len(payload), numDelivered, hasMeta), zap.Error(err))...)
		}
	}
}

// nak returns a failed message to the stream for redelivery. A nak error is a
// record of its own: the handler failure is already logged, and a lost nak
// means the message comes back only after the ack wait.
func (s *Subscriber[T]) nak(
	msg jetstream.Msg,
	log logger.Logger,
	subject string,
	numDelivered uint64,
	hasMeta bool,
) {
	if err := msg.Nak(); err != nil {
		if log != nil {
			log.Error(messaging.EventNakFailed,
				append(s.recordFields(subject, len(msg.Data()), numDelivered, hasMeta), zap.Error(err))...)
		}
	}
}

// recordFields assembles the identity fields of one consume record. The payload
// is never among them: only its size is safe to log. Each call returns a fresh
// slice, so a caller may append its error field without touching another
// record.
func (s *Subscriber[T]) recordFields(
	subject string,
	payloadBytes int,
	numDelivered uint64,
	hasMeta bool,
) []zap.Field {
	fields := []zap.Field{
		zap.String("stream", s.stream),
		zap.String("consumer", s.consName),
		zap.String("subject", subject),
		zap.Int("payload_bytes", payloadBytes),
	}
	if hasMeta {
		fields = append(fields, zap.Uint64("num_delivered", numDelivered))
	}
	return fields
}

func (s *Subscriber[T]) Unsubscribe() error {
	if s.cancel != nil {
		s.cancel()
	}

	if s.consCtx != nil {
		s.consCtx.Stop()
	}

	return nil
}
