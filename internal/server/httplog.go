package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
)

// requestIDHeader is the HTTP spelling of the correlation id.
const requestIDHeader = "X-Request-Id"

// maxRequestIDHeader rejects an absurd client-supplied id before it reaches a
// log line or a response header.
const maxRequestIDHeader = 64

// forwardRequestIDHeader lets a caller-supplied X-Request-Id reach the gRPC
// metadata, where the identity interceptor adopts it. Every other header keeps
// the gateway default, so nothing else changes behaviour.
func forwardRequestIDHeader(key string) (string, bool) {
	if strings.EqualFold(key, requestIDHeader) {
		return interceptor.RequestIDMetadataKey, true
	}
	return runtime.DefaultHeaderMatcher(key)
}

// withRequestLog mints or adopts the correlation id for every plain HTTP route
// (gateway RPCs, /logtohook, /v1/mods/archive.zip), echoes it to the client and
// records the visible outcome. One REST call therefore produces an
// http.call.finish record and a grpc.call.finish record under the same id.
func (s *Server) withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := adoptRequestID(r.Header.Get(requestIDHeader))
		w.Header().Set(requestIDHeader, rid)

		ctx := logging.InjectFields(r.Context(), logging.Fields{"request_id", rid})

		// A websocket upgrade hijacks the connection, so the writer stays
		// unwrapped; the id and its record still apply.
		if isWebsocketUpgrade(r) {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r.WithContext(ctx))

		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}

		logger.From(ctx, s.log).Info("finished http request",
			zap.String("event", "http.call.finish"),
			zap.String("component", "gateway"),
			zap.String("http_method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status", status),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.String("remote_addr", r.RemoteAddr),
			zap.String("user_agent", r.UserAgent()),
		)
	})
}

// adoptRequestID keeps a valid client id so a proxy or browser session can
// carry one id end to end, and mints a time-ordered one otherwise.
func adoptRequestID(incoming string) string {
	incoming = strings.TrimSpace(incoming)
	if incoming != "" && len(incoming) <= maxRequestIDHeader {
		if _, err := uuid.Parse(incoming); err == nil {
			return incoming
		}
	}

	rid, err := uuid.NewV7()
	if err != nil {
		// A missing id must not fail the request: the record is still useful
		// with a readable fallback.
		return "noid-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return rid.String()
}

func isWebsocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// statusRecorder captures the status code a handler wrote. Unwrap keeps
// http.ResponseController working, so a wrapped handler can still flush or
// hijack.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
