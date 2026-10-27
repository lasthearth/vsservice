package mjetstream

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// msgIDFromBytes derives the JetStream message id from the exact bytes going on
// the wire, so the id stays the sha256 of the published payload (the value the
// server dedupes on) and the payload is marshalled only once.
func msgIDFromBytes(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// headersSupported reports whether the connection behind js can carry message
// headers. JetStream needs a server new enough to always support them, and the
// check keeps a core-NATS connection that does not: there the publish degrades
// to a headerless one instead of failing. A nil connection reports true so the
// header logic stays exercised by fakes.
func headersSupported(js jetstream.JetStream) bool {
	nc := js.Conn()
	if nc == nil {
		return true
	}
	return nc.HeadersSupported()
}

func ensureStream(js jetstream.JetStream, name string, subjects []string) error {
	_, err := js.Stream(context.Background(), name)
	if err == nil {
		return nil
	}
	if !errors.Is(err, jetstream.ErrStreamNotFound) {
		return err
	}
	_, err = js.CreateStream(context.Background(), jetstream.StreamConfig{
		Name:      name,
		Subjects:  subjects,
		Retention: jetstream.WorkQueuePolicy,
		Storage:   jetstream.FileStorage,
		MaxAge:    7 * 24 * time.Hour,
	})
	return err
}

func ensureConsumer(js jetstream.JetStream, stream, durable, filterSubject, deliverGroup string) (jetstream.Consumer, error) {
	cfg := jetstream.ConsumerConfig{
		Durable:       durable,
		FilterSubject: filterSubject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       30 * time.Second,
		MaxDeliver:    10,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		ReplayPolicy:  jetstream.ReplayInstantPolicy,
	}
	if deliverGroup != "" {
		cfg.DeliverGroup = deliverGroup
	}
	cons, err := js.CreateOrUpdateConsumer(context.Background(), stream, cfg)
	if err != nil && !errors.Is(err, jetstream.ErrConsumerExists) {
		return nil, err
	}
	return cons, nil
}
