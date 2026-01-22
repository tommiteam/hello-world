package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	Registry *prometheus.Registry

	inFlight prometheus.Gauge
	reqTotal *prometheus.CounterVec
	reqDur   *prometheus.HistogramVec
}

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
	}

	// Useful default collectors (Go + process)
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.inFlight,
		m.reqTotal,
		m.reqDur,
	)

	return m
}

// MetricsHandler returns a /metrics handler bound to this app registry (no global default registry).
func (m *Metrics) MetricsHandler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}

// Middleware instruments handlers.
// route should be a stable string like "/", "/healthz".
func (m *Metrics) Middleware(route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.inFlight.Inc()
		defer m.inFlight.Dec()

		start := time.Now()
		srw := &statusRecordingWriter{ResponseWriter: w, status: 200}

		next.ServeHTTP(srw, r)

		code := strconv.Itoa(srw.status)
		m.reqTotal.WithLabelValues(route, r.Method, code).Inc()
		m.reqDur.WithLabelValues(route, r.Method).Observe(time.Since(start).Seconds())
	})
}

type statusRecordingWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusRecordingWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
