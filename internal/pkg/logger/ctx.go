package logger

import (
	"context"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"go.uber.org/zap"
)

type ctxLoggerKey struct{}

// WithLogger stores l in ctx so From returns it without rebuilding the
// fields. Handlers may attach their own logger this way; the interceptors
// rely on the middleware fields instead.
func WithLogger(ctx context.Context, l Logger) context.Context {
	return context.WithValue(ctx, ctxLoggerKey{}, l)
}

// From returns the context logger: the one stored by WithLogger, or fallback
// enriched with the correlation fields the gRPC middleware put in ctx
// (request_id, user_id). With neither present it returns fallback unchanged,
// so call sites migrate one by one without an ordering constraint.
func From(ctx context.Context, fallback Logger) Logger {
	if fallback == nil {
		return nil
	}
	if ctx != nil {
		if l, ok := ctx.Value(ctxLoggerKey{}).(Logger); ok && l != nil {
			return l
		}
	}
	fields := ctxFields(ctx)
	if len(fields) == 0 {
		return fallback
	}
	return fallback.With(fields...)
}

// RequestID returns the correlation id the interceptors put in ctx, or an
// empty string when the call carries none (background jobs before they mint
// one, unit tests). Async producers use it to keep a chain under one id.
func RequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	pairs := logging.ExtractFields(ctx)
	for i := 0; i+1 < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok || key != "request_id" {
			continue
		}
		if rid, ok := pairs[i+1].(string); ok {
			return rid
		}
	}
	return ""
}

// ctxFields converts the middleware's context log fields (key/value pairs)
// into zap fields, so an application logger carries the same correlation ids
// the access log does.
func ctxFields(ctx context.Context) []zap.Field {
	if ctx == nil {
		return nil
	}
	pairs := logging.ExtractFields(ctx)
	if len(pairs) == 0 {
		return nil
	}

	out := make([]zap.Field, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			continue
		}
		out = append(out, toField(key, pairs[i+1]))
	}
	return out
}

// toField maps a middleware field value to the closest zap field type.
func toField(key string, v any) zap.Field {
	switch val := v.(type) {
	case string:
		return zap.String(key, val)
	case int:
		return zap.Int(key, val)
	case int64:
		return zap.Int64(key, val)
	case uint64:
		return zap.Uint64(key, val)
	case bool:
		return zap.Bool(key, val)
	case time.Duration:
		return zap.Duration(key, val)
	default:
		return zap.Any(key, val)
	}
}
