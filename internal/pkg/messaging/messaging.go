package messaging

import (
	"context"
	"errors"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

const DefaultQueueGroup = "workers"

type QueueResponse[T any] struct {
	Data T
}

type (
	QueueSubscribeCallback[Req, Resp any] func(ctx context.Context, data Req) (Resp, error)
	QueueObserveCallback[Req any]         func(ctx context.Context, data Req)
)

type Queue[Req, Resp any] interface {
	Publish(ctx context.Context, data Req) error
	Request(ctx context.Context, data Req) (*QueueResponse[Resp], error)
	SubscribeGroup(queueGroup string, data QueueSubscribeCallback[Req, Resp], obs ...QueueObserveCallback[Req]) error
	Subscribe(data QueueSubscribeCallback[Req, Resp]) error
	Unsubscribe()
}

const lhNatsErrorHeader = "LHError"

func (c *NatsQueue[Req, Resp]) Request(ctx context.Context, req Req) (*QueueResponse[Resp], error) {
	reqBytes, err := natsEncode(c.encoder, req)
	if err != nil {
		return nil, err
	}

	msg := &nats.Msg{
		Subject: c.subject,
		Reply:   nats.NewInbox(),
		Header:  nats.Header{},
		Data:    reqBytes,
	}

	var retries int
	startTime := time.Now()

	for retries < 10 || (c.timeout.Seconds() != 0 && time.Since(startTime) < c.timeout) {
		resp, err := c.nc.RequestMsgWithContext(ctx, msg)
		if err != nil {
			if errors.Is(err, nats.ErrNoResponders) {
				retries++
				time.Sleep(1 * time.Second)
				continue
			}

			return nil, err
		}

		if errMsg := resp.Header.Get(lhNatsErrorHeader); errMsg != "" {
			return nil, errors.New(errMsg)
		}

		res, err := natsDecode[Resp](c.encoder, resp.Data)
		if err != nil {
			retries++
			continue
		}

		return &QueueResponse[Resp]{
			Data: res,
		}, nil
	}

	return nil, errors.New("timeout")
}

func (c *NatsQueue[Req, Resp]) SubscribeGroup(
	queueGroup string,
	cb QueueSubscribeCallback[Req, Resp],
	obs ...QueueObserveCallback[Req],
) error {
	sub, err := c.nc.QueueSubscribe(
		c.subject,
		queueGroup,
		func(requestMsg *nats.Msg) {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
				defer cancel()
				resp := &nats.Msg{
					Subject: requestMsg.Reply,
					Header:  nats.Header{},
				}

				data, err := natsDecode[Req](c.encoder, requestMsg.Data)
				if err != nil {
					if c.log != nil {
						c.log.Error("failed to decode request", zap.Error(err))
					}
					resp.Header.Set(lhNatsErrorHeader, err.Error())
					c.nc.PublishMsg(resp)
					return
				}

				response, err := cb(ctx, data)
				if err != nil {
					if c.log != nil {
						c.log.Error("failed to process request", zap.Error(err))
					}
					resp.Header.Set(lhNatsErrorHeader, err.Error())
					c.nc.PublishMsg(resp)
					return
				}

				for _, observer := range obs {
					observer(ctx, data)
				}

				responseBytes, err := natsEncode(c.encoder, response)
				if err != nil {
					if c.log != nil {
						c.log.Error("failed to encode response", zap.Error(err))
					}
					resp.Header.Set(lhNatsErrorHeader, err.Error())
					c.nc.PublishMsg(resp)
					return
				}

				resp.Data = responseBytes

				c.nc.PublishMsg(resp)
			}()
		},
	)

	c.subscription = sub

	return err
}

func (c *NatsQueue[Req, Resp]) Subscribe(
	cb QueueSubscribeCallback[Req, Resp],
) error {
	sub, err := c.nc.Subscribe(
		c.subject,
		func(m *nats.Msg) {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
				defer cancel()

				resp := &nats.Msg{
					Subject: m.Reply,
					Header:  nats.Header{},
				}

				data, err := natsDecode[Req](c.encoder, m.Data)
				if err != nil {
					if c.log != nil {
						c.log.Error("failed to decode request", zap.Error(err))
					}
					resp.Header.Set(lhNatsErrorHeader, err.Error())
					c.nc.PublishMsg(resp)
					return
				}

				response, err := cb(ctx, data)
				if err != nil {
					if c.log != nil {
						c.log.Error("failed to process request", zap.Error(err))
					}
					resp.Header.Set(lhNatsErrorHeader, err.Error())
					c.nc.PublishMsg(resp)
					return
				}

				responseBytes, err := natsEncode(c.encoder, response)
				if err != nil {
					if c.log != nil {
						c.log.Error("failed to encode response", zap.Error(err))
					}
					resp.Header.Set(lhNatsErrorHeader, err.Error())
					c.nc.PublishMsg(resp)
					return
				}

				resp.Data = responseBytes

				c.nc.PublishMsg(resp)
			}()
		},
	)

	c.subscription = sub

	return err
}

func (c *NatsQueue[Req, Resp]) Publish(ctx context.Context, data Req) error {
	dataBytes, err := natsEncode(c.encoder, data)
	if err != nil {
		return err
	}

	msg := &nats.Msg{
		Subject: c.subject,
		Header:  nats.Header{},
		Data:    dataBytes,
		Sub:     nil,
	}

	return c.nc.PublishMsg(msg)
}

func (c *NatsQueue[Req, Resp]) Unsubscribe() {
	if c.subscription != nil {
		err := c.subscription.Unsubscribe()
		if err != nil {
			c.log.Error("failed to unsubscribe", zap.Error(err))
		}
	}
}
