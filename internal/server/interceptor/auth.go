package interceptor

import (
	"context"
	"slices"
	"strings"

	middleware "github.com/grpc-ecosystem/go-grpc-middleware/v2"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// bearerPrefix includes the separating space: matching on "Bearer" alone lets a
// header value of exactly that word through, and the token slice that follows
// then reads past the end of the string.
const bearerPrefix = "Bearer "

func (interceptor *Auth) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, err := interceptor.authorize(ctx, info.FullMethod)
		if err != nil {
			return ctx, err
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
		ctx, err := interceptor.authorize(stream.Context(), info.FullMethod)
		if err != nil {
			return err
		}

		wrapped := middleware.WrapServerStream(stream)
		wrapped.WrappedContext = ctx
		return handler(srv, wrapped)
	}
}

func (interceptor *Auth) authorize(ctx context.Context, method string) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx, interceptor.deny(ctx, method, "no_metadata",
			status.Errorf(codes.Unauthenticated, "metadata is not provided"))
	}

	values := md["authorization"]
	if len(values) == 0 {
		return ctx, interceptor.deny(ctx, method, "no_token",
			status.Errorf(codes.Unauthenticated, "authorization token is not provided"))
	}

	accessToken := values[0]

	// CutPrefix requires the trailing space, so a bare "Bearer" is rejected
	// here instead of panicking on the slice that used to follow.
	token, ok := strings.CutPrefix(accessToken, bearerPrefix)
	if !ok || token == "" {
		return ctx, interceptor.deny(ctx, method, "bad_token_format",
			status.Errorf(codes.Unauthenticated, "invalid authorization token format"))
	}

	claims, err := interceptor.jwtManager.Verify(token)
	if err != nil {
		return ctx, interceptor.deny(ctx, method, "invalid_token",
			status.Errorf(codes.Unauthenticated, "access token is invalid: %v", err))
	}

	ctx, err = provideReqID(ctx)
	if err != nil {
		return ctx, err
	}
	ctx, err = provideUserID(ctx, claims.Subject)
	if err != nil {
		return ctx, err
	}
	ctx, err = provideClaims(ctx, claims)
	if err != nil {
		return ctx, err
	}

	if requiredScope, ok := interceptor.policy[Method(method)]; ok {
		// ScopeAuthenticated means the method declares itself deliberately
		// scope-free, so a valid token is enough.
		if requiredScope == ScopeAuthenticated {
			return ctx, nil
		}
		// strings.Fields, not strings.Split: Split(" ") on an empty or
		// space-padded claim yields empty tokens, so an empty required scope
		// used to match a scope-less token while denying every real one.
		if slices.Contains(strings.Fields(claims.Scope), string(requiredScope)) {
			return ctx, nil
		}
		return ctx, interceptor.deny(ctx, method, "missing_scope:"+string(requiredScope),
			status.Errorf(codes.PermissionDenied, "no permission to access this resource"))
	}

	// Methods absent from the policy table are authenticated but not
	// scope-checked: fail-open by omission. Deliberately unchanged — the
	// alternative requires a Logto scope for every method.
	return ctx, nil
}

// deny logs one record per refused call and returns the status unchanged. The
// log names the reason; the response stays as generic as it was, and neither
// carries the token.
func (interceptor *Auth) deny(ctx context.Context, method, reason string, err error) error {
	fields := []zap.Field{
		zap.String("event", "grpc.auth.denied"),
		zap.String("component", "grpc"),
		zap.String("reason", reason),
		zap.String("method", method),
	}
	if rid, ridErr := GetRequestID(ctx); ridErr == nil {
		fields = append(fields, zap.String("request_id", rid))
	}
	if uid, uidErr := GetUserID(ctx); uidErr == nil && uid != "" {
		fields = append(fields, zap.String("user_id", uid))
	}

	interceptor.log.Warn("grpc call denied", fields...)
	return err
}
