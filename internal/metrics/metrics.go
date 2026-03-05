package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/felixge/httpsnoop"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics encapsulates all Prometheus metrics for the service.
type Metrics struct {
	Registry *prometheus.Registry

	inFlight     prometheus.Gauge
	reqTotal     *prometheus.CounterVec
	reqDur       *prometheus.HistogramVec
	redisUp      prometheus.Gauge
	redisPingDur prometheus.Histogram
}

// New creates a new Metrics instance with a dedicated Prometheus registry.
func New() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		Registry: reg,

		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "hello_http_in_flight_requests",
			Help: "In-flight HTTP requests.",
		}),

		reqTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "hello_http_requests_total",
				Help: "Total number of HTTP requests.",
			},
			[]string{"route", "method", "code"},
		),

		reqDur: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "hello_http_request_duration_seconds",
				Help:    "HTTP request latency in seconds.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"route", "method"},
		),

		redisUp: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "hello_redis_up",
			Help: "Redis reachable (1=up, 0=down).",
		}),

		redisPingDur: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "hello_redis_ping_duration_seconds",
			Help:    "Redis ping latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}),
	}

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.inFlight,
		m.reqTotal,
		m.reqDur,
		m.redisUp,
		m.redisPingDur,
	)

	return m
}

// MetricsHandler returns an http.Handler for the /metrics endpoint.
func (m *Metrics) MetricsHandler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}

// SetRedisUp sets the redis_up gauge.
func (m *Metrics) SetRedisUp(up bool) {
	if up {
		m.redisUp.Set(1)
		return
	}
	m.redisUp.Set(0)
}

// ObserveRedisPing records a Redis ping latency.
func (m *Metrics) ObserveRedisPing(seconds float64) {
	m.redisPingDur.Observe(seconds)
}

// Middleware wraps an http.Handler with request metrics recording.
func (m *Metrics) Middleware(route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.inFlight.Inc()
		defer m.inFlight.Dec()

		start := time.Now()
		cm := httpsnoop.CaptureMetrics(next, w, r)

		code := strconv.Itoa(cm.Code)
		m.reqTotal.WithLabelValues(route, r.Method, code).Inc()
		m.reqDur.WithLabelValues(route, r.Method).Observe(time.Since(start).Seconds())
	})
}
