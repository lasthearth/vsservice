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
	data, err := natsEncode(r.encoder, req)
	if err != nil {
		return *new(Resp), err
	}
	msg := &nats.Msg{Subject: r.subject, Reply: nats.NewInbox(), Data: data}

	respMsg, err := r.nc.RequestMsgWithContext(ctx, msg)
	if err != nil {
		return *new(Resp), err
	}

	resp, err := natsDecode[Resp](r.encoder, respMsg.Data)
	if err != nil {
		return *new(Resp), err
	}
	return resp, nil
}

func (r *rpcResponder[Req, Resp]) Respond(handler func(ctx context.Context, req Req) (Resp, error)) error {
	msgHandler := func(m *nats.Msg) {
		ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
		defer cancel()

		req, err := natsDecode[Req](r.encoder, m.Data)
		if err != nil {
			if r.log != nil {
				r.log.Error("request decode failed", zap.Error(err))
			}
			return
		}

		resp, err := handler(ctx, req)
		if err != nil {
			if r.log != nil {
				r.log.Error("handler failed", zap.Error(err))
			}
			return
		}

		data, err := natsEncode(r.encoder, resp)
		if err != nil {
			return
		}
		r.nc.Publish(m.Reply, data)
	}

	var err error
	r.sub, err = r.nc.QueueSubscribe(r.subject, r.queueGroup, msgHandler)
	return err
}

func (r *rpcResponder[Req, Resp]) Unsubscribe() error {
	if r.sub != nil {
		return r.sub.Unsubscribe()
	}
	return nil
}
