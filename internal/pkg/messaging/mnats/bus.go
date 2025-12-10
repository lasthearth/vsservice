package mnats

import (
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type natsBus struct {
	nc      *nats.Conn
	subject string
	log     *zap.Logger
	timeout time.Duration
	encoder QueueEncoder
	sub     *nats.Subscription
}

func defaultNatsBus(nc *nats.Conn, subject string) *natsBus {
	return &natsBus{
		nc:      nc,
		subject: subject,
		timeout: 30 * time.Second,
		encoder: JsonEncoder,
	}
}

type BusOption func(*natsBus)

func WithLogger(l *zap.Logger) BusOption {
	return func(b *natsBus) {
		b.log = l
	}
}

func WithEncoder(e QueueEncoder) BusOption {
	return func(b *natsBus) {
		b.encoder = e
	}
}

func WithTimeout(d time.Duration) BusOption {
	return func(b *natsBus) {
		b.timeout = d
	}
}
