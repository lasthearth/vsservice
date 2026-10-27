package mjetstream

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type testEvent struct {
	KitID    string `json:"kit_id"`
	PlayerID string `json:"player_id"`
}

// undecodableEvent cannot be marshalled by encoding/json, so it drives the
// encode-failure record.
type undecodableEvent struct {
	Ch chan int `json:"ch"`
}

const (
	testStream   = "KIT"
	testSubject  = "kit.granted"
	testConsumer = "kit-claimed-consumer"
	testInbox    = "_INBOX.test"
)

// testPayload is the exact wire format the publisher must not change: the
// correlation id travels in a header instead.
var testPayload = []byte(`{"kit_id":"k1","player_id":"p1"}`)

// fakeJS stands in for a JetStream context. The embedded interface stays nil,
// so a call the code under test must not make panics instead of passing
// silently.
type fakeJS struct {
	jetstream.JetStream

	conn      *nats.Conn
	published []*nats.Msg
	pubErr    error
	seq       uint64
}

func (f *fakeJS) Conn() *nats.Conn { return f.conn }

func (f *fakeJS) PublishMsg(
	_ context.Context,
	m *nats.Msg,
	_ ...jetstream.PublishOpt,
) (*jetstream.PubAck, error) {
	f.published = append(f.published, m)
	if f.pubErr != nil {
		return nil, f.pubErr
	}
	f.seq++
	return &jetstream.PubAck{Stream: testStream, Sequence: f.seq}, nil
}

// fakeMsg stands in for a delivered JetStream message. Only the parts the
// subscriber reads are modelled; the ack and nak calls are counted so a test
// can assert which one ran.
type fakeMsg struct {
	subject string
	data    []byte
	headers nats.Header
	meta    *jetstream.MsgMetadata
	metaErr error
	ackErr  error
	nakErr  error
	acks    int
	naks    int
}

var _ jetstream.Msg = (*fakeMsg)(nil)

func (m *fakeMsg) Metadata() (*jetstream.MsgMetadata, error) { return m.meta, m.metaErr }
func (m *fakeMsg) Data() []byte                              { return m.data }
func (m *fakeMsg) Headers() nats.Header                      { return m.headers }
func (m *fakeMsg) Subject() string                           { return m.subject }
func (m *fakeMsg) Reply() string                             { return testInbox }

func (m *fakeMsg) Ack() error {
	m.acks++
	return m.ackErr
}

func (m *fakeMsg) DoubleAck(context.Context) error {
	m.acks++
	return m.ackErr
}

func (m *fakeMsg) Nak() error {
	m.naks++
	return m.nakErr
}

func (m *fakeMsg) NakWithDelay(time.Duration) error {
	m.naks++
	return m.nakErr
}

func (m *fakeMsg) InProgress() error           { return nil }
func (m *fakeMsg) Term() error                 { return nil }
func (m *fakeMsg) TermWithReason(string) error { return nil }
