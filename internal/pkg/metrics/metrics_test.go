package metrics

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	method       = "/donate.v1.DonateService/AddCoins"
	requestsName = "vsservice_grpc_requests_total"
	durationName = "vsservice_grpc_request_duration_seconds"
)

// One call must move both series, and the bucket set is part of the contract:
// the plan reads p99 over a 5 m window, so a boundary that drifted would
// silently flatten the tail.
func TestRecordGRPCCountsAndObserves(t *testing.T) {
	m := New(config.Config{})

	m.RecordGRPC(method, "OK", 250*time.Millisecond)
	m.RecordGRPC(method, "OK", 250*time.Millisecond)

	wantCounter := `
# HELP vsservice_grpc_requests_total Finished gRPC calls by method and status code.
# TYPE vsservice_grpc_requests_total counter
vsservice_grpc_requests_total{code="OK",method="/donate.v1.DonateService/AddCoins"} 2
`
	if err := testutil.CollectAndCompare(m.grpcRequests, strings.NewReader(wantCounter)); err != nil {
		t.Error(err)
	}

	// 0.25 s lands in the le="0.25" bucket, which is the boundary the tail
	// alert depends on: a call exactly at a boundary must be inside it.
	wantHistogram := `
# HELP vsservice_grpc_request_duration_seconds Duration of finished gRPC calls in seconds, by method and status code.
# TYPE vsservice_grpc_request_duration_seconds histogram
vsservice_grpc_request_duration_seconds_bucket{code="OK",method="/donate.v1.DonateService/AddCoins",le="0.005"} 0
vsservice_grpc_request_duration_seconds_bucket{code="OK",method="/donate.v1.DonateService/AddCoins",le="0.01"} 0
vsservice_grpc_request_duration_seconds_bucket{code="OK",method="/donate.v1.DonateService/AddCoins",le="0.025"} 0
vsservice_grpc_request_duration_seconds_bucket{code="OK",method="/donate.v1.DonateService/AddCoins",le="0.05"} 0
vsservice_grpc_request_duration_seconds_bucket{code="OK",method="/donate.v1.DonateService/AddCoins",le="0.1"} 0
vsservice_grpc_request_duration_seconds_bucket{code="OK",method="/donate.v1.DonateService/AddCoins",le="0.25"} 2
vsservice_grpc_request_duration_seconds_bucket{code="OK",method="/donate.v1.DonateService/AddCoins",le="0.5"} 2
vsservice_grpc_request_duration_seconds_bucket{code="OK",method="/donate.v1.DonateService/AddCoins",le="1"} 2
vsservice_grpc_request_duration_seconds_bucket{code="OK",method="/donate.v1.DonateService/AddCoins",le="2.5"} 2
vsservice_grpc_request_duration_seconds_bucket{code="OK",method="/donate.v1.DonateService/AddCoins",le="5"} 2
vsservice_grpc_request_duration_seconds_bucket{code="OK",method="/donate.v1.DonateService/AddCoins",le="10"} 2
vsservice_grpc_request_duration_seconds_bucket{code="OK",method="/donate.v1.DonateService/AddCoins",le="+Inf"} 2
vsservice_grpc_request_duration_seconds_sum{code="OK",method="/donate.v1.DonateService/AddCoins"} 0.5
vsservice_grpc_request_duration_seconds_count{code="OK",method="/donate.v1.DonateService/AddCoins"} 2
`
	if err := testutil.CollectAndCompare(m.grpcDuration, strings.NewReader(wantHistogram)); err != nil {
		t.Error(err)
	}
}

// method and code are the two labels and nothing else: a third label would
// multiply the series count by every value it can take.
func TestLabelSetIsMethodAndCode(t *testing.T) {
	m := New(config.Config{})

	m.RecordGRPC(method, "OK", time.Millisecond)
	m.RecordGRPC("/news.v1.NewsService/ListNews", "Internal", time.Millisecond)

	if got := testutil.CollectAndCount(m.grpcRequests); got != 2 {
		t.Errorf("want 2 request series, got %d", got)
	}
	if got := testutil.CollectAndCount(m.grpcDuration); got != 2 {
		t.Errorf("want 2 duration series, got %d", got)
	}
}

// A scrape must carry the runtime and process series. They are pre-registered
// on the default registry, which this package does not use, so they only
// appear because New registers them here.
func TestRegistryCarriesRuntimeCollectors(t *testing.T) {
	m := New(config.Config{})

	families, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	names := map[string]bool{}
	for _, mf := range families {
		names[mf.GetName()] = true
	}

	if !names["go_goroutines"] {
		t.Error("want the Go runtime collector registered")
	}
	var process bool
	for name := range names {
		if strings.HasPrefix(name, "process_") {
			process = true
			break
		}
	}
	if !process {
		t.Errorf("want the process collector registered, got families: %v", names)
	}
}

// The registry is private: a collector another package put in the global
// default registry must not appear in this service's scrape.
func TestRegistryIgnoresTheDefaultRegistry(t *testing.T) {
	m := New(config.Config{})

	stray := prometheus.NewCounter(prometheus.CounterOpts{Name: "stray_from_another_package"})
	if err := prometheus.DefaultRegisterer.Register(stray); err != nil {
		t.Fatalf("register a stray counter: %v", err)
	}
	t.Cleanup(func() { prometheus.DefaultRegisterer.Unregister(stray) })

	if got, err := testutil.GatherAndCount(m.Registry(), "stray_from_another_package"); err != nil || got != 0 {
		t.Errorf("want the private registry to ignore the default one, got %d series and %v", got, err)
	}
}

// Callers that build the interceptor without metrics must not panic.
func TestNilMetricsRecordsNothing(t *testing.T) {
	var m *Metrics

	m.RecordGRPC(method, "OK", time.Second)

	if got := m.Registry(); got != nil {
		t.Errorf("want a nil registry from nil Metrics, got %v", got)
	}
}

// The scrape endpoint answers 200 and carries the recorded series.
func TestHandlerServesTheRecordedSeries(t *testing.T) {
	m := New(config.Config{})
	m.RecordGRPC(method, "OK", 12*time.Millisecond)

	srv := httptest.NewServer(m.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer func() {
		if _, drainErr := io.Copy(io.Discard, resp.Body); drainErr != nil {
			t.Errorf("drain the response body: %v", drainErr)
		}
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("close the response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read the body: %v", err)
	}
	if !strings.Contains(string(body), requestsName) {
		t.Errorf("want %s in the scrape, got:\n%s", requestsName, body)
	}
	if !strings.Contains(string(body), durationName) {
		t.Errorf("want %s in the scrape, got:\n%s", durationName, body)
	}
}

// A container deployment must override the loopback default: inside a container
// nothing outside it can reach a loopback listener, so the Prometheus container
// on the overlay network would never scrape it.
func TestRunBindsTheConfiguredAddress(t *testing.T) {
	port := freeLoopbackPort(t)

	m := New(config.Config{MetricsPort: port, MetricsBindAddr: "0.0.0.0"})
	m.RecordGRPC(method, "OK", 5*time.Millisecond)

	lc := &fakeLifecycle{}
	Run(lc, m, testLogger(t))

	if err := lc.hooks[0].OnStart(context.Background()); err != nil {
		t.Fatalf("OnStart: %v", err)
	}
	defer func() {
		if err := lc.hooks[0].OnStop(context.Background()); err != nil {
			t.Errorf("OnStop: %v", err)
		}
	}()

	// An all-interfaces listener answers on loopback too, which is what makes
	// the scrape reachable from both inside and outside the container.
	resp, err := http.Get("http://" + net.JoinHostPort(defaultBindAddr, strconv.Itoa(port)) + "/metrics")
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close body: %v", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// A port already in use must fail startup rather than surface later as a log
// line from a goroutine nobody watches. The message names the address the
// listener bound, which is loopback unless METRICS_BIND_ADDR says otherwise.
func TestRunFailsWhenThePortIsTaken(t *testing.T) {
	// Held open for the whole test: the point is that the port is busy.
	taken, err := net.Listen("tcp", net.JoinHostPort(defaultBindAddr, "0"))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() {
		if closeErr := taken.Close(); closeErr != nil {
			t.Errorf("close the probe listener: %v", closeErr)
		}
	}()

	addr := taken.Addr().String()

	lc := &fakeLifecycle{}
	Run(lc, New(config.Config{MetricsPort: taken.Addr().(*net.TCPAddr).Port}), testLogger(t))

	if len(lc.hooks) != 1 {
		t.Fatalf("want one lifecycle hook, got %d", len(lc.hooks))
	}

	startErr := lc.hooks[0].OnStart(context.Background())
	if startErr == nil {
		t.Fatal("want OnStart to fail on a busy port, got nil")
	}
	if !strings.Contains(startErr.Error(), addr) {
		t.Errorf("want the error to name %s, got: %v", addr, startErr)
	}
}

// The listener answers on the loopback address Run binds, and OnStop closes it.
func TestRunStartsAndStopsTheListener(t *testing.T) {
	port := freeLoopbackPort(t)
	addr := net.JoinHostPort(defaultBindAddr, strconv.Itoa(port))

	m := New(config.Config{MetricsPort: port})
	// An empty CounterVec has no series at all, so a scrape of a service that
	// served no call yet carries no vsservice_* line: record one first.
	m.RecordGRPC(method, "OK", 12*time.Millisecond)

	lc := &fakeLifecycle{}
	Run(lc, m, testLogger(t))

	if len(lc.hooks) != 1 {
		t.Fatalf("want one lifecycle hook, got %d", len(lc.hooks))
	}

	if err := lc.hooks[0].OnStart(context.Background()); err != nil {
		t.Fatalf("OnStart: %v", err)
	}

	resp, err := http.Get("http://" + addr + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics on %s: %v", addr, err)
	}
	body, readErr := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); closeErr != nil {
		t.Errorf("close the response body: %v", closeErr)
	}
	if readErr != nil {
		t.Fatalf("read the body: %v", readErr)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), requestsName) {
		t.Errorf("want %s in the scrape, got:\n%s", requestsName, body)
	}

	// The mux carries one route: anything else is an unauthenticated 404.
	other, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("GET / on %s: %v", addr, err)
	}
	if closeErr := other.Body.Close(); closeErr != nil {
		t.Errorf("close the response body: %v", closeErr)
	}
	if other.StatusCode != http.StatusNotFound {
		t.Errorf("want 404 for a route other than /metrics, got %d", other.StatusCode)
	}

	if err := lc.hooks[0].OnStop(context.Background()); err != nil {
		t.Fatalf("OnStop: %v", err)
	}

	if _, dialErr := net.DialTimeout("tcp", addr, 2*time.Second); dialErr == nil {
		t.Errorf("want %s closed after OnStop, but a connection succeeded", addr)
	}
}

// freeLoopbackPort returns a port the OS just handed out and released. The
// window before Run binds it again is microseconds; a collision shows up as a
// failed OnStart rather than a wrong result.
func freeLoopbackPort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", net.JoinHostPort(defaultBindAddr, "0"))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return port
}

// fakeLifecycle captures the hooks Run registers, so a test drives the
// listener without booting the whole fx app.
type fakeLifecycle struct {
	hooks []fx.Hook
}

func (f *fakeLifecycle) Append(h fx.Hook) {
	f.hooks = append(f.hooks, h)
}

func testLogger(t *testing.T) logger.Logger {
	t.Helper()

	zc := zap.NewProductionConfig()
	// Only a failure is interesting here; the start and stop records would be
	// noise on the test output.
	zc.Level = zap.NewAtomicLevelAt(zapcore.ErrorLevel)

	l, err := logger.New(&zc)
	if err != nil {
		t.Fatalf("build the logger: %v", err)
	}
	return l
}
