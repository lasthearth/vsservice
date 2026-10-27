package mjetstream

import (
	"context"
	"encoding/json"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/nats-io/nats.go"
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
		// Tagged adds the nats component once here instead of on every record.
		// A nil logger stays nil: call sites without one keep their guard.
		log: messaging.Tagged(log),
	}, nil
}

func (p *Publisher[T]) Publish(ctx context.Context, event T) error {
	log := messaging.Log(ctx, p.log)

	data, err := json.Marshal(event)
	if err != nil {
		if log != nil {
			log.Error(messaging.EventPublishEncodeFailed,
				zap.String("stream", p.stream),
				zap.String("subject", p.subject),
				zap.Error(err))
		}
		return err
	}

	// The correlation id of the request that caused this publish rides in the
	// message header, never in the payload: external consumers parse these
	// payloads as they are today.
	msg := &nats.Msg{Subject: p.subject, Data: data}
	msg.Header = messaging.OutgoingHeader(ctx, headersSupported(p.js))

	ack, err := p.js.PublishMsg(ctx, msg, jetstream.WithMsgID(msgIDFromBytes(data)))
	if err != nil {
		if log != nil {
			log.Error(messaging.EventPublishFailed,
				zap.String("stream", p.stream),
				zap.String("subject", p.subject),
				zap.Int("payload_bytes", len(data)),
				zap.Error(err))
		}
		return err
	}

	if log != nil {
		log.Debug(messaging.EventPublishOK,
			zap.String("stream", p.stream),
			zap.String("subject", p.subject),
			zap.Int("payload_bytes", len(data)),
			// The stream sequence is the handle a replay uses to find this
			// message again.
			zap.Uint64("stream_seq", ack.Sequence))
	}
	return nil
}
