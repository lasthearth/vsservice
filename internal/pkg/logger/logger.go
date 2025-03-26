package logger

import (
	"context"
	"fmt"
	"github.com/go-faster/errors"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"go.uber.org/zap"
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
}

type logger struct {
	*zap.Config
	*zap.Logger
}

func New(cfg *zap.Config, fields ...zap.Field) (Logger, error) {
	baseLogger, err := cfg.Build()
	if err != nil {
		return nil, errors.Wrap(err, "new logger")
	}

	baseLogger = baseLogger.With(fields...)

	defer func() { _ = baseLogger.Sync() }()

	return &logger{
		Config: cfg,
		Logger: baseLogger,
	}, nil
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

		for i := 0; i < len(fields); i += 2 {
			key := fields[i]
			value := fields[i+1]

			switch v := value.(type) {
			case string:
				f = append(f, zap.String(key.(string), v))
			case int:
				f = append(f, zap.Int(key.(string), v))
			case bool:
				f = append(f, zap.Bool(key.(string), v))
			default:
				f = append(f, zap.Any(key.(string), v))
			}
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
