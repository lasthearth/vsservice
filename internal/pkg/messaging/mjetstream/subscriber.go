package mjetstream

import (
	"context"
	"encoding/json"
	"errors"
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
	consumer string
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
	if err := ensureConsumer(js, stream, consumer, subject, group); err != nil {
		return nil, err
	}

	return &Subscriber[T]{
		js:       js,
		stream:   stream,
		subject:  subject,
		consumer: consumer,
		group:    group,
		log:      log,
		timeout:  30 * time.Second,
	}, nil
}

func (s *Subscriber[T]) Subscribe(handler func(ctx context.Context, event T)) error {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	go s.pullLoop(ctx, handler)
	return nil
}

func (s *Subscriber[T]) pullLoop(parentCtx context.Context, handler func(context.Context, T)) {
	for parentCtx.Err() == nil {
		cons, err := s.js.Consumer(parentCtx, s.stream, s.consumer)
		if err != nil {
			if s.log != nil {
				s.log.Error("get consumer failed", zap.Error(err))
			}
			time.Sleep(5 * time.Second)
			continue
		}

		msgs, err := cons.Fetch(10, jetstream.FetchMaxWait(10*time.Second))
		if err != nil && !errors.Is(err, jetstream.ErrNoMessages) {
			if s.log != nil {
				s.log.Error("fetch failed", zap.Error(err))
			}
			time.Sleep(5 * time.Second)
			continue
		}

		for msg := range msgs.Messages() {
			var event T
			if err := json.Unmarshal(msg.Data(), &event); err != nil {
				if s.log != nil {
					s.log.Error("unmarshal failed", zap.Error(err))
				}
				msg.Nak()
				continue
			}

			go func(m jetstream.Msg, e T) {
				ctx, cancel := context.WithTimeout(parentCtx, s.timeout)
				defer cancel()
				handler(ctx, e)
				_ = m.Ack()
			}(msg, event)
		}
	}
}

func (s *Subscriber[T]) Unsubscribe() error {
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}
