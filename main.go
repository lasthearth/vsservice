package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/eapache/go-resiliency/retrier"
	"github.com/lasthearth/vsservice/internal/appearance"
	"github.com/lasthearth/vsservice/internal/discord"
	"github.com/lasthearth/vsservice/internal/donate"
	"github.com/lasthearth/vsservice/internal/event"
	"github.com/lasthearth/vsservice/internal/hungergames"
	"github.com/lasthearth/vsservice/internal/kitdef"
	"github.com/lasthearth/vsservice/internal/leaderboard"
	"github.com/lasthearth/vsservice/internal/lfg"
	"github.com/lasthearth/vsservice/internal/mail"
	"github.com/lasthearth/vsservice/internal/media"
	"github.com/lasthearth/vsservice/internal/mod"
	"github.com/lasthearth/vsservice/internal/news"
	"github.com/lasthearth/vsservice/internal/notification"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/httpclient"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mediaurl"
	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/pkg/tokenmanager"
	"github.com/lasthearth/vsservice/internal/player"
	"github.com/lasthearth/vsservice/internal/progression"
	"github.com/lasthearth/vsservice/internal/reaction"
	"github.com/lasthearth/vsservice/internal/referral"
	"github.com/lasthearth/vsservice/internal/rules"
	"github.com/lasthearth/vsservice/internal/server"
	serverinfo "github.com/lasthearth/vsservice/internal/serverinfo"
	"github.com/lasthearth/vsservice/internal/settlement"
	settlementtag "github.com/lasthearth/vsservice/internal/settlement-tag"
	"github.com/lasthearth/vsservice/internal/webhook"
	"github.com/lasthearth/vsservice/internal/wiki"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	// "github.com/lasthearth/vsservice/internal/trademarket"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	envDev  = "dev"
	envProd = "prod"

	serviceName = "vsservice"
)

// version is stamped at build time:
//
//	go build -ldflags "-X main.version=$(git rev-parse --short HEAD)"
//
// Every log record carries it, so a replay says which build produced it.
var version = "dev"

func main() {
	a := fx.New(
		fx.Provide(
			config.New,
			mediaurl.New,
			httpclient.New,
			func() *retrier.Retrier {
				return retrier.New(
					retrier.ConstantBackoff(5, 300*time.Millisecond),
					nil,
				)
			},
			func(client *http.Client, c config.Config) *tokenmanager.Manager {
				return tokenmanager.NewManager(client, tokenmanager.Config{
					ClientID:     c.ClientID,
					ClientSecret: c.ClientSecret,
					TokenUrl:     c.TokenUrl,
					Resource:     c.Resource,
					Scopes:       c.Scopes,
				})
			},
			messaging.SetupConn,
			setupLogger,
			setupStorage,
			mongox.New,
			mongox.NewDatabase,
		),

		leaderboard.App,
		// trademarket.App,
		rules.App,
		player.App,
		server.App,
		settlement.App,
		settlementtag.App,
		notification.App,
		news.App,
		wiki.App,
		webhook.App,
		donate.App,
		referral.App,
		hungergames.App,
		serverinfo.App,
		media.App,
		mail.App,
		kitdef.App,
		progression.App,
		discord.App,
		event.App,
		reaction.App,
		lfg.App,
		appearance.App,
		mod.App,

		fx.Invoke(registerLoggerSync),
	)

	a.Run()
}

// setupLogger builds the process logger. Every record carries the envelope
// fields service, env, version and instance, so a collector can tell which
// build on which host produced it without parsing the message.
func setupLogger(c config.Config) (logger.Logger, error) {
	var zc zap.Config

	switch c.AppEnv {
	case envProd:
		zc = zap.NewProductionConfig()
	case envDev:
		zc = zap.NewDevelopmentConfig()
	default:
		zc = zap.NewDevelopmentConfig()
	}

	// LOG_FORMAT overrides the env-derived encoder: a prod container read by a
	// human, or a dev run feeding a collector, both need the other shape.
	switch strings.ToLower(c.LogFormat) {
	case "json":
		zc.Encoding = "json"
	case "console":
		zc.Encoding = "console"
	case "":
	default:
		return nil, fmt.Errorf("LOG_FORMAT must be json or console, got %q", c.LogFormat)
	}

	level, err := zap.ParseAtomicLevel(c.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("LOG_LEVEL: %w", err)
	}
	zc.Level = level

	// One encoder shape for both formats, so a query written against dev logs
	// works against prod logs.
	zc.EncoderConfig.TimeKey = "ts"
	zc.EncoderConfig.LevelKey = "level"
	zc.EncoderConfig.MessageKey = "msg"
	zc.EncoderConfig.NameKey = "logger"
	zc.EncoderConfig.CallerKey = "caller"
	zc.EncoderConfig.StacktraceKey = "stacktrace"
	zc.EncoderConfig.EncodeTime = zapcore.RFC3339NanoTimeEncoder
	if zc.Encoding == "json" {
		zc.EncoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
	}

	zc.OutputPaths = []string{"stdout"}
	zc.ErrorOutputPaths = []string{"stderr"}
	// Stack traces belong on errors in both formats: the dev default hides
	// them, which is where an unhandled cause used to disappear.
	zc.DisableStacktrace = false

	fields := []zap.Field{
		zap.String("service", serviceName),
		zap.String("env", c.AppEnv),
		zap.String("version", version),
	}
	if host, hostErr := os.Hostname(); hostErr == nil {
		fields = append(fields, zap.String("instance", host))
	}

	l, err := logger.New(&zc, fields...)
	if err != nil {
		return nil, err
	}

	return l.WithOptions(zap.AddStacktrace(zapcore.ErrorLevel)), nil
}

// registerLoggerSync flushes buffered records at shutdown. The constructor used
// to sync once at startup, so the last records of a dying process were lost.
func registerLoggerSync(lc fx.Lifecycle, l logger.Logger) {
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			if err := l.Sync(); err != nil {
				// Nothing left to log through a logger that just failed to
				// flush: report on stderr and keep shutting down.
				fmt.Fprintf(os.Stderr, "logger sync failed: %v\n", err)
			}
			return nil
		},
	})
}

func setupStorage(c config.Config) (*minio.Client, error) {
	client, err := minio.New(c.MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(c.MinioAccessKey, c.MinioSecretKey, ""),
		Secure: c.MinioUseSSL,
	})
	if err != nil {
		return nil, err
	}

	return client, nil
}
