package interceptor

import (
	"context"
	"slices"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func (interceptor *Auth) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		err := interceptor.authorize(ctx, info.FullMethod)
		if err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func (interceptor *Auth) Stream() grpc.StreamServerInterceptor {
	return func(
		srv any,
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		err := interceptor.authorize(stream.Context(), info.FullMethod)
		if err != nil {
			return err
		}
		return handler(srv, stream)
	}
}

func (interceptor *Auth) authorize(ctx context.Context, method string) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Errorf(codes.Unauthenticated, "metadata is not provided")
	}

	values := md["authorization"]
	if len(values) == 0 {
		return status.Errorf(codes.Unauthenticated, "authorization token is not provided")
	}

	accessToken := values[0]

	tokenIdentifier := "Bearer"
	if !strings.HasPrefix(accessToken, tokenIdentifier) {
		return status.Errorf(codes.Unauthenticated, "invalid authorization token format")
	}

	token := accessToken[len(tokenIdentifier)+1:]

	claims, err := interceptor.jwtManager.Verify(token)
	if err != nil {
		return status.Errorf(codes.Unauthenticated, "access token is invalid: %v", err)
	}

	for _, scoper := range interceptor.scopers {
		reqScopeMap := scoper.Scope()

		if requiredScope, ok := reqScopeMap[Method(method)]; ok {
			claimScopes := strings.Split(claims.Scope, " ")
			if slices.Contains(claimScopes, string(requiredScope)) {
				return nil
			}
		}
	}

	return status.Error(codes.PermissionDenied, "no permission to access this RPC")
}
