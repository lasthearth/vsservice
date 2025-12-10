package mjetstream

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func generateMsgID[T any](event T) string {
	data, _ := json.Marshal(event)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
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

func ensureConsumer(js jetstream.JetStream, stream, durable, filterSubject, deliverGroup string) error {
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
	_, err := js.CreateConsumer(context.Background(), stream, cfg)
	if err != nil && !errors.Is(err, jetstream.ErrConsumerExists) {
		return err
	}
	return nil
}
