package mjetstream

import (
	"context"
	"encoding/json"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

type Publisher[T any] struct {
	js      jetstream.JetStream
	stream  string
	subject string
	log     logger.Logger
}

func NewPublisher[T any](
	js jetstream.JetStream,
	stream, subject string,
	log logger.Logger,
) (messaging.Publisher[T], error) {
	if err := ensureStream(js, stream, []string{subject}); err != nil {
		return nil, err
	}

	return &Publisher[T]{
		js:      js,
		stream:  stream,
		subject: subject,
		log:     log,
	}, nil
}

func (p *Publisher[T]) Publish(ctx context.Context, event T) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	msgID := generateMsgID(event)
	_, err = p.js.Publish(ctx, p.subject, data, jetstream.WithMsgID(msgID))
	if err != nil && p.log != nil {
		p.log.Error("jetstream publish failed", zap.Error(err), zap.String("subject", p.subject))
	}
	return err
}
