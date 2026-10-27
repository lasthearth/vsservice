package mnats

import (
	"context"

	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type eventSubscriber[T any] struct {
	*natsBus
	queueGroup string
}

func NewEventSubscriber[T any](
	nc *nats.Conn,
	subject, queueGroup string,
	opts ...BusOption,
) messaging.Subscriber[T] {
	b := defaultNatsBus(nc, subject)
	for _, o := range opts {
		o(b)
	}
	return &eventSubscriber[T]{
		natsBus:    b,
		queueGroup: queueGroup,
	}
}

func (s *eventSubscriber[T]) Subscribe(handler func(ctx context.Context, event T) error) error {
	msgHandler := func(m *nats.Msg) {
		ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
		defer cancel()

		s.handleMsg(ctx, m, handler)
	}

	var err error
	if s.queueGroup != "" {
		s.sub, err = s.nc.QueueSubscribe(s.subject, s.queueGroup, msgHandler)
	} else {
		s.sub, err = s.nc.Subscribe(s.subject, msgHandler)
	}
	return err
}

// handleMsg decodes one delivery, runs the handler and records the outcome. It
// is split from Subscribe so the tracing rules stay testable without a live
// NATS server.
func (s *eventSubscriber[T]) handleMsg(
	ctx context.Context,
	m *nats.Msg,
	handler func(context.Context, T) error,
) {
	// The publisher put the correlation id of the originating request in the
	// header. Injecting it into ctx keeps the handler's own records, and any
	// message it publishes, under the same id.
	ctx = messaging.CtxWithRequestID(ctx, messaging.RequestIDFromHeader(m.Header))

	log := messaging.Log(ctx, s.log)
	subject := messaging.RecordSubject(m.Subject, s.subject)
	payloadBytes := len(m.Data)

	if log != nil {
		log.Debug(messaging.EventConsumeStart,
			zap.String("subject", subject),
			zap.Int("payload_bytes", payloadBytes))
	}

	event, err := natsDecode[T](s.encoder, m.Data)
	if err != nil {
		if log != nil {
			log.Error(messaging.EventDecodeFailed,
				zap.String("subject", subject),
				zap.Int("payload_bytes", payloadBytes),
				zap.Error(err))
		}
		return
	}

	if err := handler(ctx, event); err != nil {
		// Core NATS redelivers nothing, so a failed handler is always a fresh
		// fault: no delivery count exists to downgrade the record to Warn.
		if log != nil {
			log.Error(messaging.EventConsumeFailed,
				zap.String("subject", subject),
				zap.Int("payload_bytes", payloadBytes),
				zap.Error(err))
		}
		return
	}

	if log != nil {
		log.Debug(messaging.EventConsumeOK,
			zap.String("subject", subject),
			zap.Int("payload_bytes", payloadBytes))
	}
}

func (s *eventSubscriber[T]) Unsubscribe() error {
	if s.sub != nil {
		return s.sub.Unsubscribe()
	}
	return nil
}
