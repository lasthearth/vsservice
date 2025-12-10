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

func (s *eventSubscriber[T]) Subscribe(handler func(ctx context.Context, event T)) error {
	msgHandler := func(m *nats.Msg) {
		ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
		defer cancel()

		event, err := natsDecode[T](s.encoder, m.Data)
		if err != nil {
			if s.log != nil {
				s.log.Error("event decode failed", zap.Error(err), zap.String("subject", s.subject))
			}
			return
		}
		handler(ctx, event)
	}

	var err error
	if s.queueGroup != "" {
		s.sub, err = s.nc.QueueSubscribe(s.subject, s.queueGroup, msgHandler)
	} else {
		s.sub, err = s.nc.Subscribe(s.subject, msgHandler)
	}
	return err
}

func (s *eventSubscriber[T]) Unsubscribe() error {
	if s.sub != nil {
		return s.sub.Unsubscribe()
	}
	return nil
}
