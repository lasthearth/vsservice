// Package metrics owns the service's Prometheus registry and publishes it on
// a loopback-only listener.
//
// The registry is private, not prometheus.DefaultRegisterer. Any package the
// binary imports can register into the default registry, and such a collector
// would appear in this service's scrape with nobody able to say where it came
// from. A private registry keeps the scrape to exactly what this package
// registers, plus the Go runtime and process collectors.
package metrics

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	namespace = "vsservice"
	subsystem = "grpc"

	labelMethod = "method"
	labelCode   = "code"

	// defaultBindAddr is the interface the scrape listener binds when
	// METRICS_BIND_ADDR is unset. Loopback, because the endpoint carries no auth
	// and no TLS.
	//
	// A container deployment overrides it with 0.0.0.0: a listener on the
	// container's loopback is unreachable from anywhere else, including the
	// Prometheus container that scrapes it over the private overlay network. The
	// port stays unpublished, so all-interfaces inside the container is still not
	// a public interface.
	defaultBindAddr = "127.0.0.1"

	// readHeaderTimeout bounds a slow client before the handler runs. Matches
	// the gateway server, which sets it for the same reason.
	readHeaderTimeout = 10 * time.Second
)

// durationBuckets are the histogram boundaries for one finished call, in
// seconds. They follow the duration_ms scale of the grpc.call.finish access
// record — the range that record actually covers is 5 ms to 10 s — so a
// percentile read from Prometheus and a duration read from the log describe
// the same distribution. The plan's p99 window is 5 m; the 2.5/5/10 s buckets
// are what keeps a slow tail visible inside it instead of collapsing into
// +Inf.
var durationBuckets = []float64{
	0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
}

// Metrics records RED metrics for finished gRPC calls: the rate of calls by
// method and status code, their error share, and their duration distribution.
type Metrics struct {
	registry *prometheus.Registry
	port     int
	bindAddr string

	grpcRequests *prometheus.CounterVec
	grpcDuration *prometheus.HistogramVec
}

// New builds the metrics registry. cfg carries only METRICS_PORT; the
// collectors themselves take no configuration.
func New(cfg config.Config) *Metrics {
	registry := prometheus.NewRegistry()

	m := &Metrics{
		registry: registry,
		port:     cfg.MetricsPort,
		bindAddr: defaultBindAddr,
		grpcRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "requests_total",
			Help:      "Finished gRPC calls by method and status code.",
		}, []string{labelMethod, labelCode}),
		grpcDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "request_duration_seconds",
			Help:      "Duration of finished gRPC calls in seconds, by method and status code.",
			Buckets:   durationBuckets,
		}, []string{labelMethod, labelCode}),
	}

	// MustRegister rather than Register: the registry was created two lines
	// above and is never handed out for writing, so the only failure mode is a
	// duplicate name, which is a programming error worth a startup panic
	// rather than a silently missing series.
	//
	// The Go runtime and process collectors are registered explicitly. They
	// are pre-registered on prometheus.DefaultRegisterer, which this package
	// does not use, so without these a scrape would carry no go_* or
	// process_* series.
	registry.MustRegister(
		m.grpcRequests,
		m.grpcDuration,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	// An empty value means METRICS_BIND_ADDR was not set and the loopback
	// default stands; envconfig supplies the same default, so this branch only
	// matters for callers that build Config by hand.
	if cfg.MetricsBindAddr != "" {
		m.bindAddr = cfg.MetricsBindAddr
	}

	return m
}

// RecordGRPC counts one finished call and observes its duration. A nil *Metrics
// records nothing and does not panic, so callers that run without metrics
// (tests building the interceptor directly) need no guard of their own.
func (m *Metrics) RecordGRPC(method, code string, d time.Duration) {
	if m == nil {
		return
	}

	// WithLabelValues, not the Labels map form: the positional form skips a
	// map allocation on every call.
	m.grpcRequests.WithLabelValues(method, code).Inc()
	m.grpcDuration.WithLabelValues(method, code).Observe(d.Seconds())
}

// Handler returns the /metrics scrape handler for the private registry.
//
// Registry makes promhttp expose promhttp_metric_handler_errors_total in the
// same scrape, so a gather or encode failure is counted and alertable rather
// than dropped — promhttp discards those errors when ErrorLog is nil.
// ContinueOnError keeps one bad collector from failing the whole scrape.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		ErrorHandling: promhttp.ContinueOnError,
		Registry:      m.registry,
	})
}

// Registry returns the private registry so a test can Gather it. Handing it
// out is safe: the scrape contract stays "this package's series plus the
// runtime and process collectors" no matter what a caller reads from it.
func (m *Metrics) Registry() *prometheus.Registry {
	if m == nil {
		return nil
	}
	return m.registry
}

// Run publishes the scrape endpoint on the fx lifecycle.
//
// The listener binds to 127.0.0.1 on purpose: the endpoint carries no auth
// and no TLS, so it is for a Prometheus scraper on the same host, not for the
// public internet. A container deployment must publish it with a loopback
// binding on the host (127.0.0.1:6970:6970), never 0.0.0.0 — the port is not
// routed through the gateway, CORS or Traefik.
func Run(lc fx.Lifecycle, m *Metrics, log logger.Logger) {
	addr := net.JoinHostPort(m.bindAddr, strconv.Itoa(m.port))

	mux := http.NewServeMux()
	// One route and nothing else: this listener carries no auth, so a second
	// route would be a second unauthenticated surface for no benefit.
	mux.Handle("/metrics", m.Handler())

	// srv outlives OnStart so OnStop can shut down the same server. The
	// listener is created in OnStart, synchronously: a port already in use
	// must abort startup, not surface as a log line from a goroutine nobody
	// watches.
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("metrics listener on %s: %w", addr, err)
			}

			go func() {
				// Shutdown makes Serve return http.ErrServerClosed; that is
				// the stop path, not a failure.
				if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Error("metrics server stopped", zap.Error(err))
				}
			}()

			log.Info("metrics server started", zap.String("addr", addr))
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if err := srv.Shutdown(ctx); err != nil {
				return fmt.Errorf("metrics server shutdown: %w", err)
			}
			return nil
		},
	})
}
