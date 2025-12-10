package mnats

import (
	"context"

	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/nats-io/nats.go"
)

type eventPublisher[T any] natsBus

func NewEventPublisher[T any](nc *nats.Conn, subject string, opts ...BusOption) messaging.Publisher[T] {
	b := defaultNatsBus(nc, subject)
	for _, o := range opts {
		o(b)
	}
	return (*eventPublisher[T])(b)
}

func (p *eventPublisher[T]) Publish(ctx context.Context, event T) error {
	data, err := natsEncode(p.encoder, event)
	if err != nil {
		return err
	}
	return p.nc.Publish(p.subject, data)
}
