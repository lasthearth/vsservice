package interceptor

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/lasthearth/vsservice/internal/pkg/jwt"
)

type ctxKey struct {
	key string
}

// ContextWithUserID returns ctx carrying uid, also injecting it as a logging
// field. Exported so handlers can be tested from outside this package.
func ContextWithUserID(ctx context.Context, uid string) context.Context {
	ctx = logging.InjectFields(
		ctx,
		logging.Fields{"user_id", uid},
	)

	return context.WithValue(ctx, ctxKey{"sub"}, uid)
}

// ContextWithClaims returns ctx carrying claims. Exported so handlers can be
// tested from outside this package.
func ContextWithClaims(ctx context.Context, claims *jwt.Claims) context.Context {
	return context.WithValue(ctx, ctxKey{"claims"}, claims)
}

func provideUserID(ctx context.Context, uid string) (context.Context, error) {
	return ContextWithUserID(ctx, uid), nil
}

func provideClaims(ctx context.Context, payload *jwt.Claims) (context.Context, error) {
	if payload == nil {
		return nil, errors.New("payload is nil")
	}

	return ContextWithClaims(ctx, payload), nil
}

func GetClaims(ctx context.Context) (jwt.Claims, error) {
	if claims, ok := ctx.Value(ctxKey{"claims"}).(*jwt.Claims); ok {
		return *claims, nil
	}

	return jwt.Claims{}, ErrGetClaims
}

// GetUserID from context, only throws ErrGetUserID if uid not found in context
func GetUserID(ctx context.Context) (string, error) {
	if uid, ok := ctx.Value(ctxKey{"sub"}).(string); ok {
		return uid, nil
	}

	return "", ErrGetUserID
}

func provideReqID(ctx context.Context) (context.Context, error) {
	rid, err := uuid.NewV7()
	if err != nil {
		return nil, errors.New("failed to generate rid")
	}

	ctx = logging.InjectFields(
		ctx,
		logging.Fields{"request_id", rid.String()},
	)

	// Stored as a string: GetRequestID type-asserts string.
	return context.WithValue(ctx, ctxKey{"rid"}, rid.String()), nil
}

// GetRequestID from context, only throws ErrGetRequestID if uid not found in context
func GetRequestID(ctx context.Context) (string, error) {
	if rid, ok := ctx.Value(ctxKey{"rid"}).(string); ok {
		return rid, nil
	}

	return "", ErrGetRequestID
}
