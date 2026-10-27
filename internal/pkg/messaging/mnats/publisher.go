package mnats

import (
	"context"
	"errors"

	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type eventPublisher[T any] natsBus

func NewEventPublisher[T any](nc *nats.Conn, subject string, opts ...BusOption) messaging.Publisher[T] {
	b := defaultNatsBus(nc, subject)
	for _, o := range opts {
		o(b)
	}
	return (*eventPublisher[T])(b)
}

// publishMsg builds an outgoing core NATS message. The correlation id of the
// request that caused the publish rides in the header, never in the payload,
// and only when the server supports headers: an older server rejects a message
// that carries them, so the publish would fail instead of losing one field.
func publishMsg(ctx context.Context, headersOK bool, subject string, data []byte) *nats.Msg {
	msg := &nats.Msg{Subject: subject, Data: data}
	msg.Header = messaging.OutgoingHeader(ctx, headersOK)
	return msg
}

// errNoConnection replaces a panic inside the driver when a bus was built
// without a connection: a unit test or a startup race must record a failure,
// not take the process down.
var errNoConnection = errors.New("no nats connection")

// headersSupported reports whether the connection can carry message headers.
// Asking a nil connection panics, so the nil case answers false.
func headersSupported(nc *nats.Conn) bool {
	return nc != nil && nc.HeadersSupported()
}

// publish sends one fire-and-forget message with the ctx correlation header.
func publish(ctx context.Context, nc *nats.Conn, subject string, data []byte) error {
	if nc == nil {
		return errNoConnection
	}
	return nc.PublishMsg(publishMsg(ctx, headersSupported(nc), subject, data))
}

func (p *eventPublisher[T]) Publish(ctx context.Context, event T) error {
	log := messaging.Log(ctx, p.log)

	data, err := natsEncode(p.encoder, event)
	if err != nil {
		if log != nil {
			log.Error(messaging.EventPublishEncodeFailed,
				zap.String("subject", p.subject),
				zap.Error(err))
		}
		return err
	}

	if err := publish(ctx, p.nc, p.subject, data); err != nil {
		if log != nil {
			log.Error(messaging.EventPublishFailed,
				zap.String("subject", p.subject),
				zap.Int("payload_bytes", len(data)),
				zap.Error(err))
		}
		return err
	}

	if log != nil {
		log.Debug(messaging.EventPublishOK,
			zap.String("subject", p.subject),
			zap.Int("payload_bytes", len(data)))
	}
	return nil
}
