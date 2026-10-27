package interceptor

import (
	"context"
	"errors"

	"github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// internalErrorEvent marks a record whose cause was sanitized before it
// reached the client. This is the only place the original error survives.
const internalErrorEvent = "grpc.call.internal_error"

// DomainErrors maps ierror.DomainError values to proper gRPC status errors.
// Errors already wrapped in a gRPC status pass through unchanged. Any
// unrecognized non-status error becomes codes.Internal, and its full cause is
// logged first: the client message stays generic, the log keeps the detail.
type DomainErrors struct {
	log logger.Logger
}

func NewDomainErrors(log logger.Logger) *DomainErrors {
	return &DomainErrors{log: log}
}

func (d *DomainErrors) Unary() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		resp, err := handler(ctx, req)
		if err == nil {
			return resp, nil
		}

		if _, ok := status.FromError(err); ok {
			return resp, err
		}

		if de, ok := errors.AsType[*ierror.DomainError](err); ok {
			return resp, status.Error(de.Code, de.Message)
		}

		d.logInternal(ctx, info.FullMethod, err)
		return resp, InternalStatus(ctx)
	}
}

// Stream is the streaming variant.
func (d *DomainErrors) Stream() grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		err := handler(srv, ss)
		if err == nil {
			return nil
		}

		if _, ok := status.FromError(err); ok {
			return err
		}

		if de, ok := errors.AsType[*ierror.DomainError](err); ok {
			return status.Error(de.Code, de.Message)
		}

		d.logInternal(ss.Context(), info.FullMethod, err)
		return InternalStatus(ss.Context())
	}
}

// InternalStatus builds the sanitized answer for a failure the client must not
// see details of. The ref id lets a user quote the exact call in a support
// ticket; the cause stays in the log under that id.
func InternalStatus(ctx context.Context) error {
	if rid := logger.RequestID(ctx); rid != "" {
		return status.Errorf(codes.Internal, "internal server error (ref %s)", rid)
	}
	return status.Error(codes.Internal, "internal server error")
}

// logInternal records the cause an Internal response hides, with the chain and
// the correlation ids, so the visible outcome in the access record can be
// traced back to the real failure.
func (d *DomainErrors) logInternal(ctx context.Context, method string, err error) {
	logger.From(ctx, d.log).Error("unhandled error sanitized to Internal",
		zap.String("event", internalErrorEvent),
		zap.String("component", "grpc"),
		zap.String("method", method),
		zap.String("error_kind", "unhandled"),
		zap.Strings("error_chain", errorChain(err)),
		zap.Error(err),
	)
}

// errorChain unwraps err so a wrapped cause is visible even when Error() drops
// it.
func errorChain(err error) []string {
	var chain []string
	for e := err; e != nil; e = errors.Unwrap(e) {
		chain = append(chain, e.Error())
	}
	return chain
}
