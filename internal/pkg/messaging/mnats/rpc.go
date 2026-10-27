package mnats

import (
	"context"

	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type (
	rpcRequester[Req, Resp any] natsBus

	rpcResponder[Req, Resp any] struct {
		*natsBus
		queueGroup string
	}
)

func NewRpcRequester[Req, Resp any](
	nc *nats.Conn,
	subject string,
	opts ...BusOption,
) messaging.RpcRequester[Req, Resp] {
	b := defaultNatsBus(nc, subject)
	for _, o := range opts {
		o(b)
	}
	return (*rpcRequester[Req, Resp])(b)
}

func NewRpcResponder[Req, Resp any](
	nc *nats.Conn,
	subject, queueGroup string,
	opts ...BusOption,
) messaging.RpcResponder[Req, Resp] {
	b := defaultNatsBus(nc, subject)
	for _, o := range opts {
		o(b)
	}
	return &rpcResponder[Req, Resp]{
		natsBus:    b,
		queueGroup: queueGroup,
	}
}

func (r *rpcRequester[Req, Resp]) Request(ctx context.Context, req Req) (Resp, error) {
	log := messaging.Log(ctx, r.log)

	data, err := natsEncode(r.encoder, req)
	if err != nil {
		if log != nil {
			log.Error(messaging.EventPublishEncodeFailed,
				zap.String("subject", r.subject),
				zap.Error(err))
		}
		return *new(Resp), err
	}

	if r.nc == nil {
		if log != nil {
			log.Error(messaging.EventPublishFailed,
				zap.String("subject", r.subject),
				zap.Int("payload_bytes", len(data)),
				zap.Error(errNoConnection))
		}
		return *new(Resp), errNoConnection
	}

	msg := publishMsg(ctx, headersSupported(r.nc), r.subject, data)
	msg.Reply = nats.NewInbox()

	respMsg, err := r.nc.RequestMsgWithContext(ctx, msg)
	if err != nil {
		if log != nil {
			log.Error(messaging.EventPublishFailed,
				zap.String("subject", r.subject),
				zap.Int("payload_bytes", len(data)),
				zap.Error(err))
		}
		return *new(Resp), err
	}

	if log != nil {
		log.Debug(messaging.EventPublishOK,
			zap.String("subject", r.subject),
			zap.Int("payload_bytes", len(data)))
	}

	resp, err := natsDecode[Resp](r.encoder, respMsg.Data)
	if err != nil {
		if log != nil {
			// The request reached the responder, so the fault is on the way
			// back: the decode record names the reply subject the answer came
			// from instead of the request subject.
			log.Error(messaging.EventDecodeFailed,
				zap.String("subject", respMsg.Subject),
				zap.Int("payload_bytes", len(respMsg.Data)),
				zap.Error(err))
		}
		return *new(Resp), err
	}
	return resp, nil
}

func (r *rpcResponder[Req, Resp]) Respond(handler func(ctx context.Context, req Req) (Resp, error)) error {
	msgHandler := func(m *nats.Msg) {
		ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
		defer cancel()

		r.handleMsg(ctx, m, handler)
	}

	var err error
	r.sub, err = r.nc.QueueSubscribe(r.subject, r.queueGroup, msgHandler)
	return err
}

// handleMsg decodes one request, runs the handler and publishes the reply. It
// is split from Respond so the tracing rules stay testable without a live NATS
// server.
func (r *rpcResponder[Req, Resp]) handleMsg(
	ctx context.Context,
	m *nats.Msg,
	handler func(context.Context, Req) (Resp, error),
) {
	// The requester put the correlation id of its call in the header, so the
	// reply and every record of this hop stay under the same id.
	ctx = messaging.CtxWithRequestID(ctx, messaging.RequestIDFromHeader(m.Header))

	log := messaging.Log(ctx, r.log)
	subject := messaging.RecordSubject(m.Subject, r.subject)
	payloadBytes := len(m.Data)

	if log != nil {
		log.Debug(messaging.EventConsumeStart,
			zap.String("subject", subject),
			zap.Int("payload_bytes", payloadBytes))
	}

	req, err := natsDecode[Req](r.encoder, m.Data)
	if err != nil {
		if log != nil {
			log.Error(messaging.EventDecodeFailed,
				zap.String("subject", subject),
				zap.Int("payload_bytes", payloadBytes),
				zap.Error(err))
		}
		return
	}

	resp, err := handler(ctx, req)
	if err != nil {
		if log != nil {
			log.Error(messaging.EventConsumeFailed,
				zap.String("subject", subject),
				zap.Int("payload_bytes", payloadBytes),
				zap.Error(err))
		}
		return
	}

	if log != nil {
		log.Debug(messaging.EventConsumeOK,
			zap.String("subject", subject),
			zap.Int("payload_bytes", payloadBytes))
	}

	data, err := natsEncode(r.encoder, resp)
	if err != nil {
		if log != nil {
			// The handler succeeded but its answer cannot go on the wire; the
			// requester sees a timeout, so the loss is recorded here.
			log.Error(messaging.EventPublishEncodeFailed,
				zap.String("subject", m.Reply),
				zap.Error(err))
		}
		return
	}

	if err := publish(ctx, r.nc, m.Reply, data); err != nil {
		if log != nil {
			log.Error(messaging.EventPublishFailed,
				zap.String("subject", m.Reply),
				zap.Int("payload_bytes", len(data)),
				zap.Error(err))
		}
		return
	}

	if log != nil {
		log.Debug(messaging.EventPublishOK,
			zap.String("subject", m.Reply),
			zap.Int("payload_bytes", len(data)))
	}
}

func (r *rpcResponder[Req, Resp]) Unsubscribe() error {
	if r.sub != nil {
		return r.sub.Unsubscribe()
	}
	return nil
}
