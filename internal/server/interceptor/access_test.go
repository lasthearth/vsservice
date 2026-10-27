package interceptor

import (
	"context"
	"strings"
	"testing"

	eventv1 "github.com/lasthearth/vsservice/gen/event/v1"
	reactionv1 "github.com/lasthearth/vsservice/gen/reaction/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const writeMethod = "/donate.v1.DonateService/AddCoins"

// The two families the access interceptor feeds. Named here rather than read
// from the scrape text, so a rename shows up as a failing assertion instead of
// a silently absent series.
const (
	requestsFamily = "vsservice_grpc_requests_total"
	durationFamily = "vsservice_grpc_request_duration_seconds"
)

// One record per call, carrying the ids from ctx: this is what makes a user
// action replayable.
func TestAccessLogCarriesCorrelationIDs(t *testing.T) {
	log, logs := newObsLogger()
	a := NewAccessLog(log, config.Config{}, nil)

	_, err := a.Unary()(ctxWithIDs("rid-1", "uid-1"), nil,
		&grpc.UnaryServerInfo{FullMethod: writeMethod},
		func(context.Context, any) (any, error) { return nil, nil })
	if err != nil {
		t.Fatalf("unary: %v", err)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record, got %d: %+v", len(entries), entries)
	}

	e := entries[0]
	if e.Level != zap.InfoLevel {
		t.Errorf("want Info for a successful call, got %v", e.Level)
	}
	if got := stringFieldOf(t, e, "event"); got != accessEvent {
		t.Errorf("want event %q, got %q", accessEvent, got)
	}
	if got := stringFieldOf(t, e, "component"); got != "grpc" {
		t.Errorf("want component grpc, got %q", got)
	}
	if got := stringFieldOf(t, e, "code"); got != "OK" {
		t.Errorf("want code OK, got %q", got)
	}
	if got := stringFieldOf(t, e, "request_id"); got != "rid-1" {
		t.Errorf("want request_id rid-1, got %q", got)
	}
	if got := stringFieldOf(t, e, "user_id"); got != "uid-1" {
		t.Errorf("want user_id uid-1, got %q", got)
	}
	if _, ok := fieldOf(e, "duration_ms"); !ok {
		t.Error("want a duration_ms field")
	}
}

// The record states the outcome the client actually saw, so an Internal answer
// is an error and a client mistake is a warning.
func TestAccessLogLevelsFollowTheOutcome(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want zapcore.Level
	}{
		{"ok", nil, zap.InfoLevel},
		{"not found", status.Error(codes.NotFound, "no such settlement"), zap.WarnLevel},
		{"permission denied", status.Error(codes.PermissionDenied, "no"), zap.WarnLevel},
		{"caller hung up", status.Error(codes.Canceled, "context canceled"), zap.WarnLevel},
		{"internal", status.Error(codes.Internal, "internal server error"), zap.ErrorLevel},
		{"unavailable", status.Error(codes.Unavailable, "try later"), zap.ErrorLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, logs := newObsLogger()
			a := NewAccessLog(log, config.Config{}, nil)

			_, err := a.Unary()(ctxWithIDs("rid-2", "uid-2"), nil,
				&grpc.UnaryServerInfo{FullMethod: writeMethod},
				func(context.Context, any) (any, error) { return nil, tt.err })
			if tt.err == nil && err != nil {
				t.Fatalf("want nil error, got %v", err)
			}

			entries := logs.All()
			if len(entries) != 1 {
				t.Fatalf("want 1 record, got %d", len(entries))
			}
			if entries[0].Level != tt.want {
				t.Errorf("want level %v, got %v", tt.want, entries[0].Level)
			}
			if tt.err != nil {
				if got := stringFieldOf(t, entries[0], "grpc_message"); got != status.Convert(tt.err).Message() {
					t.Errorf("want the visible message %q, got %q", status.Convert(tt.err).Message(), got)
				}
			}
		})
	}
}

// Sampling may only ever drop a successful read: losing a write or an error
// would break replay.
func TestAccessLogSamplesOnlySuccessfulReads(t *testing.T) {
	const method = "/news.v1.NewsService/ListNews"

	log, logs := newObsLogger()
	a := NewAccessLog(log, config.Config{LogSampleReads: 3}, nil)

	for i := 0; i < 3; i++ {
		if _, err := a.Unary()(ctxWithIDs("rid-3", ""), nil,
			&grpc.UnaryServerInfo{FullMethod: method},
			func(context.Context, any) (any, error) { return nil, nil }); err != nil {
			t.Fatalf("unary: %v", err)
		}
	}
	if got := logs.Len(); got != 1 {
		t.Errorf("want 1 of 3 sampled reads kept, got %d", got)
	}

	logs.TakeAll()
	for i := 0; i < 3; i++ {
		if _, err := a.Unary()(ctxWithIDs("rid-4", ""), nil,
			&grpc.UnaryServerInfo{FullMethod: method},
			func(context.Context, any) (any, error) {
				return nil, status.Error(codes.Internal, "boom")
			}); err == nil {
			t.Fatal("want the handler error returned")
		}
	}
	if got := logs.Len(); got != 3 {
		t.Errorf("errors are never sampled: want 3 records, got %d", got)
	}

	logs.TakeAll()
	for i := 0; i < 3; i++ {
		if _, err := a.Unary()(ctxWithIDs("rid-5", ""), nil,
			&grpc.UnaryServerInfo{FullMethod: writeMethod},
			func(context.Context, any) (any, error) { return nil, nil }); err != nil {
			t.Fatalf("unary: %v", err)
		}
	}
	if got := logs.Len(); got != 3 {
		t.Errorf("writes are never sampled: want 3 records, got %d", got)
	}
}

// Reflection and health traffic would drown the record set.
func TestAccessLogSkipsInfraMethods(t *testing.T) {
	log, logs := newObsLogger()
	a := NewAccessLog(log, config.Config{}, nil)

	_, err := a.Unary()(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo"},
		func(context.Context, any) (any, error) { return nil, nil })
	if err != nil {
		t.Fatalf("unary: %v", err)
	}
	if got := logs.Len(); got != 0 {
		t.Errorf("want no record for reflection, got %d", got)
	}
}

// The summary says which entity a call touched and how much came back, without
// carrying the payload.
func TestAccessLogSummarisesRequestAndResponse(t *testing.T) {
	log, logs := newObsLogger()
	a := NewAccessLog(log, config.Config{}, nil)

	req := &eventv1.GetEventRequest{Id: "ev-42"}
	resp := &reactionv1.ListReactionsResponse{
		Reactions: []*reactionv1.TargetReactions{{Target: "news:1"}, {Target: "news:2"}},
	}

	if _, err := a.Unary()(ctxWithIDs("rid-6", "uid-6"), req,
		&grpc.UnaryServerInfo{FullMethod: "/event.v1.EventService/GetEvent"},
		func(context.Context, any) (any, error) { return resp, nil }); err != nil {
		t.Fatalf("unary: %v", err)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record, got %d", len(entries))
	}
	if got := stringFieldOf(t, entries[0], "id"); got != "ev-42" {
		t.Errorf("want the request id field, got %q", got)
	}
	if got := stringFieldOf(t, entries[0], "resp_list"); got != "reactions" {
		t.Errorf("want resp_list reactions, got %q", got)
	}
	if f, ok := fieldOf(entries[0], "resp_count"); !ok || f.Integer != 2 {
		t.Errorf("want resp_count 2, got %+v", f)
	}
}

// A stream record must say how much the client received.
func TestAccessLogStreamCountsSentMessages(t *testing.T) {
	log, logs := newObsLogger()
	a := NewAccessLog(log, config.Config{}, nil)

	stream := &fakeServerStream{ctx: ctxWithIDs("rid-7", "uid-7")}
	err := a.Stream()(nil, stream,
		&grpc.StreamServerInfo{FullMethod: "/notification.v1.NotificationService/Watch", IsServerStream: true},
		func(_ any, ss grpc.ServerStream) error {
			for i := 0; i < 3; i++ {
				if sendErr := ss.SendMsg("msg"); sendErr != nil {
					return sendErr
				}
			}
			return nil
		})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 record, got %d", len(entries))
	}
	if f, ok := fieldOf(entries[0], "sent_msgs"); !ok || f.Integer != 3 {
		t.Errorf("want sent_msgs 3, got %+v", f)
	}
}

// A call feeds the record and the metric series with the same method and code,
// so a PromQL query and a log query describe the same call.
func TestAccessLogRecordsGRPCMetrics(t *testing.T) {
	log, _ := newObsLogger()
	m := metrics.New(config.Config{})
	a := NewAccessLog(log, config.Config{}, m)

	if _, err := a.Unary()(ctxWithIDs("rid-8", "uid-8"), nil,
		&grpc.UnaryServerInfo{FullMethod: writeMethod},
		func(context.Context, any) (any, error) { return nil, nil }); err != nil {
		t.Fatalf("unary: %v", err)
	}

	expected := `
# HELP vsservice_grpc_requests_total Finished gRPC calls by method and status code.
# TYPE vsservice_grpc_requests_total counter
vsservice_grpc_requests_total{code="OK",method="/donate.v1.DonateService/AddCoins"} 1
`
	if err := testutil.GatherAndCompare(m.Registry(), strings.NewReader(expected), requestsFamily); err != nil {
		t.Error(err)
	}

	if got := observations(t, m, writeMethod, "OK"); got != 1 {
		t.Errorf("want 1 duration observation, got %d", got)
	}
}

// Sampling exists to keep log volume down. It must not hide calls from a
// rate() or a latency percentile, so every call is counted even when its
// record is dropped.
func TestAccessLogMetricsAreNotSampled(t *testing.T) {
	const method = "/news.v1.NewsService/ListNews"

	log, logs := newObsLogger()
	m := metrics.New(config.Config{})
	a := NewAccessLog(log, config.Config{LogSampleReads: 3}, m)

	for i := 0; i < 3; i++ {
		if _, err := a.Unary()(ctxWithIDs("rid-9", ""), nil,
			&grpc.UnaryServerInfo{FullMethod: method},
			func(context.Context, any) (any, error) { return nil, nil }); err != nil {
			t.Fatalf("unary: %v", err)
		}
	}

	if got := logs.Len(); got != 1 {
		t.Errorf("want 1 of 3 sampled reads logged, got %d", got)
	}
	if got := counterValue(t, m, method, "OK"); got != 3 {
		t.Errorf("want all 3 calls counted, got %v", got)
	}
	if got := observations(t, m, method, "OK"); got != 3 {
		t.Errorf("want all 3 durations observed, got %d", got)
	}
}

// The code label is what an error rate is read from, so an error outcome must
// be its own series rather than a swallowed one.
func TestAccessLogMetricsRecordTheCode(t *testing.T) {
	log, _ := newObsLogger()
	m := metrics.New(config.Config{})
	a := NewAccessLog(log, config.Config{}, m)

	if _, err := a.Unary()(ctxWithIDs("rid-10", ""), nil,
		&grpc.UnaryServerInfo{FullMethod: writeMethod},
		func(context.Context, any) (any, error) {
			return nil, status.Error(codes.Internal, "boom")
		}); err == nil {
		t.Fatal("want the handler error returned")
	}

	expected := `
# HELP vsservice_grpc_requests_total Finished gRPC calls by method and status code.
# TYPE vsservice_grpc_requests_total counter
vsservice_grpc_requests_total{code="Internal",method="/donate.v1.DonateService/AddCoins"} 1
`
	if err := testutil.GatherAndCompare(m.Registry(), strings.NewReader(expected), requestsFamily); err != nil {
		t.Error(err)
	}
}

// Infra traffic stays out of the metrics for the same reason it stays out of
// the log: a health probe every second would dominate the request rate.
func TestAccessLogSkipsInfraMetrics(t *testing.T) {
	log, _ := newObsLogger()
	m := metrics.New(config.Config{})
	a := NewAccessLog(log, config.Config{}, m)

	if _, err := a.Unary()(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"},
		func(context.Context, any) (any, error) { return nil, nil }); err != nil {
		t.Fatalf("unary: %v", err)
	}

	if got, err := testutil.GatherAndCount(m.Registry(), requestsFamily); err != nil || got != 0 {
		t.Errorf("want no request series for a health probe, got %d series and %v", got, err)
	}
}

// A stream call lands in the same series a unary call does.
func TestAccessLogStreamRecordsMetrics(t *testing.T) {
	log, _ := newObsLogger()
	m := metrics.New(config.Config{})
	a := NewAccessLog(log, config.Config{}, m)

	const method = "/notification.v1.NotificationService/Watch"

	stream := &fakeServerStream{ctx: ctxWithIDs("rid-11", "uid-11")}
	err := a.Stream()(nil, stream,
		&grpc.StreamServerInfo{FullMethod: method, IsServerStream: true},
		func(_ any, ss grpc.ServerStream) error {
			return ss.SendMsg("msg")
		})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	expected := `
# HELP vsservice_grpc_requests_total Finished gRPC calls by method and status code.
# TYPE vsservice_grpc_requests_total counter
vsservice_grpc_requests_total{code="OK",method="/notification.v1.NotificationService/Watch"} 1
`
	if err := testutil.GatherAndCompare(m.Registry(), strings.NewReader(expected), requestsFamily); err != nil {
		t.Error(err)
	}
	if got := observations(t, m, method, "OK"); got != 1 {
		t.Errorf("want 1 duration observation, got %d", got)
	}
}

// Tests build the interceptor without metrics; recording must record nothing
// and not panic, for a unary and a stream call alike.
func TestAccessLogNilMetricsDoesNotPanic(t *testing.T) {
	log, logs := newObsLogger()
	a := NewAccessLog(log, config.Config{}, nil)

	if _, err := a.Unary()(ctxWithIDs("rid-12", ""), nil,
		&grpc.UnaryServerInfo{FullMethod: writeMethod},
		func(context.Context, any) (any, error) { return nil, nil }); err != nil {
		t.Fatalf("unary: %v", err)
	}

	stream := &fakeServerStream{ctx: ctxWithIDs("rid-13", "")}
	if err := a.Stream()(nil, stream,
		&grpc.StreamServerInfo{FullMethod: "/notification.v1.NotificationService/Watch", IsServerStream: true},
		func(any, grpc.ServerStream) error { return nil }); err != nil {
		t.Fatalf("stream: %v", err)
	}

	if got := logs.Len(); got != 2 {
		t.Errorf("want both calls logged, got %d", got)
	}
}

// series reads one method/code pair out of the registry. Only the field that
// matches the family asked for carries a value: a counter series has no
// histogram and the other way round, and both stay zero when the pair was
// never recorded, which is exactly what "not instrumented" looks like.
//
// The client_model types are never named here: Gather returns them and the
// getters are enough, so this file does not import a package the service
// itself does not use.
func series(t *testing.T, m *metrics.Metrics, family, method, code string) (counter float64, observations uint64) {
	t.Helper()

	families, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	for _, mf := range families {
		if mf.GetName() != family {
			continue
		}
		for _, metric := range mf.GetMetric() {
			labels := map[string]string{}
			for _, pair := range metric.GetLabel() {
				labels[pair.GetName()] = pair.GetValue()
			}
			if labels["method"] != method || labels["code"] != code {
				continue
			}
			// The generated getters are nil-safe, so both can be read as they
			// are and the unused one comes back zero.
			return metric.GetCounter().GetValue(), metric.GetHistogram().GetSampleCount()
		}
	}
	return 0, 0
}

// counterValue returns how many calls a method/code pair counted.
func counterValue(t *testing.T, m *metrics.Metrics, method, code string) float64 {
	t.Helper()
	value, _ := series(t, m, requestsFamily, method, code)
	return value
}

// observations returns how many calls a method/code pair recorded a duration
// for.
func observations(t *testing.T, m *metrics.Metrics, method, code string) uint64 {
	t.Helper()
	_, count := series(t, m, durationFamily, method, code)
	return count
}
