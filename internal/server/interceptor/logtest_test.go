package interceptor

import (
	"context"
	"testing"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// fakeServerStream is the minimum a stream interceptor needs: a context, and
// Send/SetHeader calls the counting wrapper can observe.
type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *fakeServerStream) Context() context.Context     { return s.ctx }
func (s *fakeServerStream) SendMsg(any) error            { return nil }
func (s *fakeServerStream) RecvMsg(any) error            { return nil }
func (s *fakeServerStream) SetHeader(metadata.MD) error  { return nil }
func (s *fakeServerStream) SendHeader(metadata.MD) error { return nil }
func (s *fakeServerStream) SetTrailer(metadata.MD)       {}

// obsLogger adapts a zap observer core to the project Logger interface, so a
// test can assert on the exact records an interceptor emits instead of parsing
// output.
type obsLogger struct {
	*zap.Logger
}

func newObsLogger() (logger.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zap.DebugLevel)
	return obsLogger{Logger: zap.New(core)}, logs
}

func (l obsLogger) With(fields ...zap.Field) logger.Logger {
	return obsLogger{Logger: l.Logger.With(fields...)}
}

func (l obsLogger) Named(name string) logger.Logger {
	return obsLogger{Logger: l.Logger.Named(name)}
}

func (l obsLogger) WithScope(scope string) logger.Logger {
	return l.With(zap.String("scope", scope))
}

func (l obsLogger) WithMethod(method string) logger.Logger {
	return l.With(zap.String("method", method))
}

func (l obsLogger) WithComponent(component string) logger.Logger {
	return l.With(zap.String("component", component))
}

func (l obsLogger) WithOptions(opts ...zap.Option) logger.Logger {
	return obsLogger{Logger: l.Logger.WithOptions(opts...)}
}

// ToLoggingLogger is unused by the interceptors under test.
func (l obsLogger) ToLoggingLogger() logging.Logger { return nil }

func (l obsLogger) Sync() error { return l.Logger.Sync() }

// ctxWithIDs returns a ctx carrying the correlation fields the identity and
// auth interceptors inject.
func ctxWithIDs(requestID, userID string) context.Context {
	return logging.InjectFields(
		context.Background(),
		logging.Fields{"request_id", requestID, "user_id", userID},
	)
}

// fieldOf returns one field of a captured record.
func fieldOf(e observer.LoggedEntry, key string) (zap.Field, bool) {
	for _, f := range e.Context {
		if f.Key == key {
			return f, true
		}
	}
	return zap.Field{}, false
}

// stringFieldOf returns one string field, failing the test when it is absent
// or of another type.
func stringFieldOf(t *testing.T, e observer.LoggedEntry, key string) string {
	t.Helper()
	f, ok := fieldOf(e, key)
	if !ok {
		t.Fatalf("record %q has no field %q: %+v", e.Message, key, e.Context)
	}
	if f.Type != zapcore.StringType {
		t.Fatalf("field %q is not a string: %+v", key, f)
	}
	return f.String
}
