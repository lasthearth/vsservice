package httpclient

import (
	"net/http"
	"regexp"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap"
)

// retryMax and requestTimeout keep the previous inline construction: five
// attempts, three seconds each.
const (
	retryMax       = 5
	requestTimeout = 3 * time.Second
)

// secretInPath matches a Telegram bot token, which lives in the URL path
// rather than in a header: api.telegram.org/bot<id>:<secret>/sendMessage.
var secretInPath = regexp.MustCompile(`bot\d+:[A-Za-z0-9_\-]{8,}`)

// secretInQuery matches a credential carried in a query value.
var secretInQuery = regexp.MustCompile(`([?&](?:token|key|api_key|apikey|secret|signature|password|auth)=)([^&\s]+)`)

// maskSecrets removes credentials from a URL-derived string before it reaches
// a log record. The query is never logged by the hooks, but the library's own
// messages embed whole URLs.
func maskSecrets(s string) string {
	s = secretInPath.ReplaceAllString(s, "bot[redacted]")
	return secretInQuery.ReplaceAllString(s, "${1}[redacted]")
}

// New builds the shared outbound client. With HTTP_CLIENT_LOG on, every
// attempt, response and final failure is recorded under the correlation id of
// the calling request; retryablehttp otherwise retries five times in silence.
func New(log logger.Logger, c config.Config) *http.Client {
	client := retryablehttp.NewClient()
	client.RetryMax = retryMax
	client.HTTPClient.Timeout = requestTimeout

	if !c.HTTPClientLog {
		// nil keeps the library's own stderr logger out of the process log.
		client.Logger = nil
		return client.StandardClient()
	}

	hl := log.Named("http_client")
	client.Logger = leveledAdapter{log: hl}
	client.RequestLogHook = requestHook(hl)
	client.ResponseLogHook = responseHook(hl)
	client.ErrorHandler = errorHook(hl)

	return client.StandardClient()
}

// leveledAdapter routes retryablehttp's own messages (retry waits, transport
// errors) into the application log with stable keys.
type leveledAdapter struct {
	log logger.Logger
}

func (a leveledAdapter) Error(msg string, kv ...any) { a.log.Error(maskSecrets(msg), kvFields(kv)...) }
func (a leveledAdapter) Warn(msg string, kv ...any)  { a.log.Warn(maskSecrets(msg), kvFields(kv)...) }
func (a leveledAdapter) Info(msg string, kv ...any)  { a.log.Info(maskSecrets(msg), kvFields(kv)...) }
func (a leveledAdapter) Debug(msg string, kv ...any) { a.log.Debug(maskSecrets(msg), kvFields(kv)...) }

func kvFields(kv []any) []zap.Field {
	out := make([]zap.Field, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		key, ok := kv[i].(string)
		if !ok {
			continue
		}
		out = append(out, kvField(key, kv[i+1]))
	}
	return out
}

func kvField(key string, v any) zap.Field {
	switch val := v.(type) {
	case string:
		return zap.String(key, maskSecrets(val))
	case error:
		return zap.NamedError(key, val)
	case int:
		return zap.Int(key, val)
	case int64:
		return zap.Int64(key, val)
	case bool:
		return zap.Bool(key, val)
	default:
		return zap.Any(key, val)
	}
}

func requestHook(log logger.Logger) retryablehttp.RequestLogHook {
	return func(_ retryablehttp.Logger, req *http.Request, attempt int) {
		logger.From(req.Context(), log).Debug("http request started",
			zap.String("event", "http.out"),
			zap.String("component", "http"),
			zap.Int("attempt", attempt+1),
			zap.String("http_method", req.Method),
			zap.String("host", req.URL.Host),
			zap.String("path", maskSecrets(req.URL.Path)),
		)
	}
}

func responseHook(log logger.Logger) retryablehttp.ResponseLogHook {
	return func(_ retryablehttp.Logger, resp *http.Response) {
		fields := []zap.Field{
			zap.String("event", "http.out.response"),
			zap.String("component", "http"),
			zap.Int("status", resp.StatusCode),
		}

		l := log
		if req := resp.Request; req != nil {
			l = logger.From(req.Context(), log)
			if req.URL != nil {
				fields = append(fields,
					zap.String("host", req.URL.Host),
					zap.String("path", maskSecrets(req.URL.Path)),
				)
			}
		}

		l.Debug("http response", fields...)
	}
}

func errorHook(log logger.Logger) retryablehttp.ErrorHandler {
	return func(resp *http.Response, err error, numTries int) (*http.Response, error) {
		fields := []zap.Field{
			zap.String("event", "http.out.failed"),
			zap.String("component", "http"),
			zap.Int("tries", numTries),
			zap.Error(err),
		}
		if resp != nil && resp.Request != nil && resp.Request.URL != nil {
			fields = append(fields,
				zap.String("host", resp.Request.URL.Host),
				zap.String("path", maskSecrets(resp.Request.URL.Path)),
			)
		}

		log.Error("http request failed after retries", fields...)

		if resp != nil && resp.Body != nil {
			if cerr := resp.Body.Close(); cerr != nil {
				log.Warn("failed to close the response body after retries",
					zap.String("event", "http.out.close_failed"),
					zap.Error(cerr),
				)
			}
		}
		return resp, err
	}
}
