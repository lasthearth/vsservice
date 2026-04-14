package interceptor

import (
	"context"
	"errors"

	"github.com/lasthearth/vsservice/internal/pkg/ierror"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DomainErrorUnaryInterceptor maps ierror.DomainError values to proper gRPC
// status errors. Errors already wrapped in a gRPC status pass through unchanged.
// Any unrecognized non-status error becomes codes.Internal.
func DomainErrorUnaryInterceptor(
	ctx context.Context,
	req any,
	_ *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	resp, err := handler(ctx, req)
	if err == nil {
		return resp, nil
	}

	if _, ok := status.FromError(err); ok {
		return resp, err
	}

	var de *ierror.DomainError
	if errors.As(err, &de) {
		return resp, status.Error(de.Code, de.Message)
	}

	return resp, status.Error(codes.Internal, "internal server error")
}

// DomainErrorStreamInterceptor is the streaming variant.
func DomainErrorStreamInterceptor(
	srv any,
	ss grpc.ServerStream,
	_ *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	err := handler(srv, ss)
	if err == nil {
		return nil
	}

	if _, ok := status.FromError(err); ok {
		return err
	}

	var de *ierror.DomainError
	if errors.As(err, &de) {
		return status.Error(de.Code, de.Message)
	}

	return status.Error(codes.Internal, "internal server error")
}
