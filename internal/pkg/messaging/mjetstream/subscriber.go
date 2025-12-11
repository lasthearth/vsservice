package mjetstream

import (
	"context"
	"encoding/json"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

type Subscriber[T any] struct {
	js       jetstream.JetStream
	stream   string
	subject  string
	consName string
	cons     jetstream.Consumer
	consCtx  jetstream.ConsumeContext
	group    string
	log      logger.Logger
	timeout  time.Duration
	cancel   context.CancelFunc
}

func NewSubscriber[T any](
	js jetstream.JetStream,
	stream, subject, consumer, group string,
	log logger.Logger,
) (messaging.Subscriber[T], error) {
	if err := ensureStream(js, stream, []string{subject}); err != nil {
		return nil, err
	}
	cons, err := ensureConsumer(js, stream, consumer, subject, group)
	if err != nil {
		return nil, err
	}

	return &Subscriber[T]{
		js:       js,
		stream:   stream,
		subject:  subject,
		consName: consumer,
		cons:     cons,
		group:    group,
		log:      log,
		timeout:  30 * time.Second,
	}, nil
}

func (s *Subscriber[T]) Subscribe(handler func(ctx context.Context, event T) error) error {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	consCtx, err := s.cons.Consume(func(msg jetstream.Msg) {
		var event T
		if err := json.Unmarshal(msg.Data(), &event); err != nil {
			if s.log != nil {
				s.log.Error("unmarshal failed", zap.Error(err))
			}
			msg.Nak()
		}

		if err := handler(ctx, event); err != nil {
			if s.log != nil {
				s.log.Error("handler failed", zap.Error(err))
			}
			msg.Nak()
		}

		msg.Ack()
	})
	if err != nil {
		return err
	}

	s.consCtx = consCtx
	return nil
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
