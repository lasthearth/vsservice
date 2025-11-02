package messaging

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/nats-io/nats.go"
)

func SetupConn(cfg config.Config) (*nats.Conn, error) {
	nc, err := nats.Connect(cfg.NatsUrl)
	if err != nil {
		return nil, err
	}

	return nc, nil
}

type QueueOptsFn func(opts *QueueOpts)

type QueueOpts struct {
	timeout time.Duration
	encoder QueueEncoder
	log     logger.Logger
}

type NatsQueue[Req, Resp any] struct {
	QueueOpts
	nc           *nats.Conn
	subscription *nats.Subscription
	subject      string
}

func NewNatsQueue[Req, Resp any](
	nc *nats.Conn,
	subject string,
	fns ...QueueOptsFn,
) *NatsQueue[Req, Resp] {
	opts := QueueOpts{
		timeout: time.Second * 5,
		encoder: JsonEncoder,
		log:     nil,
	}
	for _, fn := range fns {
		fn(&opts)
	}

	return &NatsQueue[Req, Resp]{
		QueueOpts: opts,
		nc:        nc,
		subject:   subject,
	}
}

func WithTimeout(timeout time.Duration) QueueOptsFn {
	return func(opts *QueueOpts) {
		opts.timeout = timeout
	}
}

func WithEncoder(encoder QueueEncoder) QueueOptsFn {
	return func(opts *QueueOpts) {
		opts.encoder = encoder
	}
}

func WithLogger(log logger.Logger) QueueOptsFn {
	return func(opts *QueueOpts) {
		opts.log = log
	}
}
