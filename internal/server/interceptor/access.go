package interceptor

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	middleware "github.com/grpc-ecosystem/go-grpc-middleware/v2"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/metrics"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// accessEvent is the stable key every access record carries. Dashboards and
// alerts filter on it, never on the human message.
const accessEvent = "grpc.call.finish"

// infraServicePrefixes are not domain calls: reflection traffic and health
// probes would drown the record set.
var infraServicePrefixes = []string{
	"/grpc.reflection.",
	"/grpc.health.",
}

// sampledReadPrefixes are the successful read shapes that may be sampled.
// Writes, errors and warnings are never sampled.
var sampledReadPrefixes = []string{"List", "Get", "WorldTime", "TotalOnline"}

// maxSummaryFields caps how many request/response fields one record carries.
const maxSummaryFields = 5

// AccessLog writes one record per call: the visible outcome (code, message,
// duration) plus the correlation ids the identity and auth interceptors put in
// ctx. It replaces the disabled go-grpc-middleware logging interceptor,
// because the level, the sampling and the summary rules are ours.
//
// The same call also feeds the Prometheus counters and histograms: one code
// path computes method, code and duration, so the log record and the metric
// series can never disagree about what a call cost.
type AccessLog struct {
	log         logger.Logger
	metrics     *metrics.Metrics
	sampleReads int
	counter     atomic.Uint64
}

// NewAccessLog builds the interceptor. m may be nil — every method on
// *metrics.Metrics is nil-safe, so a build that does not wire metrics (tests)
// still logs.
func NewAccessLog(log logger.Logger, c config.Config, m *metrics.Metrics) *AccessLog {
	return &AccessLog{
		log:         log,
		metrics:     m,
		sampleReads: c.LogSampleReads,
	}
}

func (a *AccessLog) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		a.record(ctx, info.FullMethod, start, req, resp, 0, err)
		return resp, err
	}
}

func (a *AccessLog) Stream() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()

		wrapped := middleware.WrapServerStream(stream)
		counting := &countingStream{ServerStream: wrapped}

		err := handler(srv, counting)
		a.record(wrapped.WrappedContext, info.FullMethod, start, nil, nil, counting.sent, err)
		return err
	}
}

// record emits the access record and the metric series for one finished call.
// Sampling applies only to a successful call of a read-shaped method, and only
// to the log: metrics see every call, because a rate() or a latency percentile
// over a sampled subset is wrong rather than merely less detailed.
func (a *AccessLog) record(ctx context.Context, method string, start time.Time, req, resp any, sent int, err error) {
	// Infra traffic is excluded from both sinks: a health probe every second
	// would drown the record set and dominate the request rate.
	if isInfraMethod(method) {
		return
	}

	code := status.Code(err)
	duration := time.Since(start)

	a.metrics.RecordGRPC(method, code.String(), duration)

	if code == codes.OK && a.sampled(method) {
		return
	}

	fields := []zap.Field{
		zap.String("event", accessEvent),
		zap.String("component", "grpc"),
		zap.String("code", code.String()),
		zap.Int64("duration_ms", duration.Milliseconds()),
	}
	if err != nil {
		// The message the client actually received: DomainError already
		// sanitized it, so this is the visible outcome, not the cause.
		fields = append(fields, zap.String("grpc_message", status.Convert(err).Message()))
	}
	if sent > 0 {
		fields = append(fields, zap.Int("sent_msgs", sent))
	}
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		fields = append(fields, zap.String("peer", p.Addr.String()))
	}
	fields = append(fields, clientFields(ctx)...)
	fields = append(fields, requestSummary(req)...)
	fields = append(fields, responseSummary(resp)...)

	log := logger.From(ctx, a.log)
	switch levelFor(code) {
	case zap.DebugLevel:
		log.Debug("finished call", fields...)
	case zap.WarnLevel:
		log.Warn("finished call", fields...)
	case zap.ErrorLevel:
		log.Error("finished call", fields...)
	default:
		log.Info("finished call", fields...)
	}
}

// levelFor maps the visible outcome to a level: a client mistake is a warning,
// a server failure is an error.
func levelFor(code codes.Code) zapcore.Level {
	switch code {
	case codes.OK:
		return zap.InfoLevel
	case codes.InvalidArgument, codes.NotFound, codes.PermissionDenied,
		codes.Unauthenticated, codes.Aborted, codes.FailedPrecondition:
		return zap.WarnLevel
	case codes.Canceled:
		// The caller hung up: nothing failed on this side, and a page of these
		// at Error would hide a real outage.
		return zap.WarnLevel
	default:
		return zap.ErrorLevel
	}
}

// sampled reports whether this call is dropped from the log. One record in
// sampleReads survives; sampleReads of 0 or 1 keeps everything.
func (a *AccessLog) sampled(method string) bool {
	if a.sampleReads <= 1 {
		return false
	}
	if !isSampledRead(method) {
		return false
	}
	return a.counter.Add(1)%uint64(a.sampleReads) != 0
}

func isSampledRead(method string) bool {
	name := methodName(method)
	for _, prefix := range sampledReadPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func isInfraMethod(method string) bool {
	for _, prefix := range infraServicePrefixes {
		if strings.HasPrefix(method, prefix) {
			return true
		}
	}
	return false
}

// methodName returns the last segment of a full gRPC method.
func methodName(method string) string {
	if i := strings.LastIndex(method, "/"); i >= 0 && i+1 < len(method) {
		return method[i+1:]
	}
	return method
}

// clientFields records who called, from the metadata the gateway forwards.
func clientFields(ctx context.Context) []zap.Field {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil
	}

	var fields []zap.Field
	if ua := firstValue(md, "user-agent", "grpcgateway-user-agent"); ua != "" {
		fields = append(fields, zap.String("user_agent", ua))
	}
	if xff := firstValue(md, "x-forwarded-for", "grpcgateway-x-forwarded-for"); xff != "" {
		fields = append(fields, zap.String("client_ip", xff))
	}
	return fields
}

func firstValue(md metadata.MD, keys ...string) string {
	for _, key := range keys {
		if values := md.Get(key); len(values) > 0 && values[0] != "" {
			return values[0]
		}
	}
	return ""
}

// countingStream counts the messages a stream handler sent, so a stream record
// says how much the client received.
type countingStream struct {
	grpc.ServerStream
	sent int
}

func (c *countingStream) SendMsg(m any) error {
	err := c.ServerStream.SendMsg(m)
	if err == nil {
		c.sent++
	}
	return err
}
