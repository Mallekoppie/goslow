package platform

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

var (
	metricsOnce     sync.Once
	metricsRegistry *prometheus.Registry
	metricsActive   atomic.Bool

	metricsListenerLock    sync.Mutex
	metricsListenerRunning bool

	httpRequestsTotal   *prometheus.CounterVec
	httpRequestDuration *prometheus.HistogramVec
	rpcRequestsTotal    *prometheus.CounterVec
	rpcRequestDuration  *prometheus.HistogramVec
	authDecisionsTotal  *prometheus.CounterVec
	inflightRequests    *prometheus.GaugeVec
)

// initializeMetrics builds the dedicated registry once and ties recording to the
// active configuration. The registry is process-lifetime: disabling metrics
// stops recording, and re-enabling resumes into the same families.
func initializeMetrics(cfg *Config) {
	if cfg == nil || !cfg.Metrics.Enabled {
		metricsActive.Store(false)
		return
	}

	metricsOnce.Do(func() {
		registry := prometheus.NewRegistry()

		httpRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "goslow_http_requests_total",
			Help: "Total HTTP requests handled, by method, route, and status.",
		}, []string{"method", "route", "status"})
		httpRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "goslow_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"})
		rpcRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "goslow_rpc_requests_total",
			Help: "Total RPCs handled, by transport, procedure, and code.",
		}, []string{"transport", "procedure", "code"})
		rpcRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "goslow_rpc_request_duration_seconds",
			Help:    "RPC duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"transport", "procedure"})
		authDecisionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "goslow_auth_decisions_total",
			Help: "Authentication decisions by transport and outcome.",
		}, []string{"transport", "outcome"})
		inflightRequests = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "goslow_inflight_requests",
			Help: "Requests currently in flight by transport.",
		}, []string{"transport"})

		registry.MustRegister(
			httpRequestsTotal,
			httpRequestDuration,
			rpcRequestsTotal,
			rpcRequestDuration,
			authDecisionsTotal,
			inflightRequests,
			collectors.NewGoCollector(),
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		)

		metricsRegistry = registry
	})

	metricsActive.Store(true)
}

func metricsEnabled() bool {
	return metricsActive.Load() && metricsRegistry != nil
}

// MetricsHandler exposes the dedicated registry. It is intended to be served on
// the isolated metrics listener.
func MetricsHandler() http.Handler {
	if !metricsEnabled() {
		return http.NotFoundHandler()
	}
	return promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{})
}

// StartMetricsServerWithContext serves /metrics on metrics.listenaddress until
// the context is cancelled. It is a no-op when metrics are disabled.
func StartMetricsServerWithContext(ctx context.Context) error {
	conf, err := GetPlatformConfiguration()
	if err != nil {
		return err
	}
	if conf == nil || !conf.Metrics.Enabled {
		return nil
	}
	if err := validateListenAddress("metrics.listenaddress", conf.Metrics.ListenAddress); err != nil {
		return err
	}

	initializeMetrics(conf)

	listener, err := net.Listen("tcp", conf.Metrics.ListenAddress)
	if err != nil {
		return err
	}
	defer listener.Close()

	path := conf.Metrics.Path
	if path == "" {
		path = "/metrics"
	}

	mux := http.NewServeMux()
	mux.Handle(path, MetricsHandler())

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	Log.Info("Starting metrics server", zap.String("addr", conf.Metrics.ListenAddress), zap.String("path", path))
	return serveHTTPServer(ctx, server, listener, false, "", "")
}

// startMetricsServerIfEnabled launches the isolated metrics listener alongside a
// server. Repeated calls while a listener is running are no-ops, so configuration
// changes or repeated server starts cannot bind the address twice.
func startMetricsServerIfEnabled(ctx context.Context, conf *Config) {
	if conf == nil || !conf.Metrics.Enabled {
		return
	}
	initializeMetrics(conf)

	metricsListenerLock.Lock()
	if metricsListenerRunning {
		metricsListenerLock.Unlock()
		return
	}
	metricsListenerRunning = true
	metricsListenerLock.Unlock()

	go func() {
		defer func() {
			metricsListenerLock.Lock()
			metricsListenerRunning = false
			metricsListenerLock.Unlock()
		}()
		if err := StartMetricsServerWithContext(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			Log.Error("metrics server stopped", zap.Error(err))
		}
	}()
}

func observeHTTPRequest(method, route string, status int, duration time.Duration) {
	if !metricsEnabled() {
		return
	}
	httpRequestsTotal.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
	httpRequestDuration.WithLabelValues(method, route).Observe(duration.Seconds())
}

func observeRPCRequest(transport, procedure, code string, duration time.Duration) {
	if !metricsEnabled() {
		return
	}
	rpcRequestsTotal.WithLabelValues(transport, procedure, code).Inc()
	rpcRequestDuration.WithLabelValues(transport, procedure).Observe(duration.Seconds())
}

func recordAuthDecision(transport, outcome string) {
	if !metricsEnabled() {
		return
	}
	authDecisionsTotal.WithLabelValues(transport, outcome).Inc()
}

func trackInflight(transport string) func() {
	if !metricsEnabled() {
		return func() {}
	}
	inflightRequests.WithLabelValues(transport).Inc()
	return func() { inflightRequests.WithLabelValues(transport).Dec() }
}
