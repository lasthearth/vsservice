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

func provideUserID(ctx context.Context, payload jwt.Claims) (context.Context, error) {
	ctx = logging.InjectFields(
		ctx,
		logging.Fields{"user_id", payload.Subject},
	)

	return context.WithValue(ctx, ctxKey{"sub"}, payload.Subject), nil
}

func provideClaims(ctx context.Context, payload jwt.Claims) (context.Context, error) {
	return context.WithValue(ctx, ctxKey{"claims"}, payload), nil
}

func GetClaims(ctx context.Context) (jwt.Claims, error) {
	if claims, ok := ctx.Value(ctxKey{"claims"}).(jwt.Claims); ok {
		return claims, nil
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

	return context.WithValue(ctx, ctxKey{"rid"}, rid), nil
}

// GetRequestID from context, only throws ErrGetRequestID if uid not found in context
func GetRequestID(ctx context.Context) (string, error) {
	if rid, ok := ctx.Value(ctxKey{"rid"}).(string); ok {
		return rid, nil
	}

	return "", ErrGetRequestID
}
