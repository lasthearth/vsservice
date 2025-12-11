package messaging

import (
	"context"
)

const DefaultWorkerGroup = "workers"

type Publisher[T any] interface {
	Publish(ctx context.Context, event T) error
}

type Subscriber[T any] interface {
	Subscribe(handler func(ctx context.Context, event T) error) error
	Unsubscribe() error
}

type RpcRequester[Req, Resp any] interface {
	Request(ctx context.Context, req Req) (Resp, error)
}

type RpcResponder[Req, Resp any] interface {
	Respond(handler func(ctx context.Context, req Req) (Resp, error)) error
	Unsubscribe() error
}
