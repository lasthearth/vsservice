package logger

import (
	"context"
	"fmt"

	"github.com/go-faster/errors"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger interface {
	Debug(msg string, fields ...zap.Field)
	Info(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)

	With(fields ...zap.Field) Logger
	Named(string) Logger
	WithScope(scope string) Logger
	WithMethod(method string) Logger
	WithOptions(opts ...zap.Option) Logger
	WithComponent(component string) Logger

	ToLoggingLogger() logging.Logger

	// Sync flushes buffered entries. Called from the fx OnStop hook: the
	// constructor used to sync once at startup, so shutdown records could be
	// lost.
	Sync() error
}

type logger struct {
	*zap.Config
	*zap.Logger
}

func New(cfg *zap.Config, fields ...zap.Field) (Logger, error) {
	baseLogger, err := cfg.Build(zap.WrapCore(func(c zapcore.Core) zapcore.Core {
		return redactingCore{Core: c}
	}))
	if err != nil {
		return nil, errors.Wrap(err, "new logger")
	}

	baseLogger = baseLogger.With(fields...)

	return &logger{
		Config: cfg,
		Logger: baseLogger,
	}, nil
}

// NewWithCore builds a Logger on an arbitrary zap core, behind the same
// redaction New applies. It exists so tests can capture records in memory
// without opening a file sink, which Windows keeps locked for the life of the
// process.
func NewWithCore(core zapcore.Core) Logger {
	return &logger{
		Config: &zap.Config{},
		Logger: zap.New(redactingCore{Core: core}),
	}
}

func (l *logger) Named(name string) Logger {
	return &logger{
		Config: l.Config,
		Logger: l.Logger.Named(name),
	}
}

func (l *logger) With(fields ...zap.Field) Logger {
	return &logger{
		Config: l.Config,
		Logger: l.Logger.With(fields...),
	}
}

func (l *logger) WithMethod(method string) Logger {
	return &logger{
		Config: l.Config,
		Logger: l.Logger.With(zap.String("method", method)),
	}
}

func (l *logger) WithScope(scope string) Logger {
	return &logger{
		Config: l.Config,
		Logger: l.Logger.With(zap.String("scope", scope)),
	}
}

func (l *logger) WithOptions(opts ...zap.Option) Logger {
	return &logger{
		Config: l.Config,
		Logger: l.Logger.WithOptions(opts...),
	}
}

func (l *logger) WithComponent(component string) Logger {
	return &logger{
		Config: l.Config,
		Logger: l.Logger.With(zap.String("component", component)),
	}
}

func (l *logger) ToLoggingLogger() logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		f := make([]zap.Field, 0, len(fields)/2)

		for i := 0; i+1 < len(fields); i += 2 {
			key, ok := fields[i].(string)
			if !ok {
				continue
			}
			f = append(f, toField(key, fields[i+1]))
		}

		log := l.WithOptions(zap.AddCallerSkip(1)).With(f...)

		switch lvl {
		case logging.LevelDebug:
			log.Debug(msg)
		case logging.LevelInfo:
			log.Info(msg)
		case logging.LevelWarn:
			log.Warn(msg)
		case logging.LevelError:
			log.Error(msg)
		default:
			panic(fmt.Sprintf("unknown level %v", lvl))
		}
	})
}
