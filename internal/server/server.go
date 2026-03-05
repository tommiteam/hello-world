// Package server provides the HTTP server, routes, and middleware.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"helloapp/internal/metrics"
	redisclient "helloapp/internal/redis"
)

// Server wraps the standard http.Server with application dependencies.
type Server struct {
	httpSrv *http.Server
	metrics *metrics.Metrics
	redis   *redisclient.Client
}

// New creates a new Server with all routes wired.
func New(addr string, m *metrics.Metrics, rc *redisclient.Client) *Server {
	s := &Server{
		metrics: m,
		redis:   rc,
	}

	mux := http.NewServeMux()

	// Business endpoint
	mux.Handle("/", m.Middleware("/", http.HandlerFunc(s.handleRoot)))

	// Health endpoints
	mux.Handle("/healthz", http.HandlerFunc(s.handleHealthz))
	mux.Handle("/readyz", http.HandlerFunc(s.handleHealthz)) // alias
	mux.Handle("/livez", http.HandlerFunc(s.handleLivez))

	// Debug / test
	mux.Handle("/boom", m.Middleware("/boom", http.HandlerFunc(s.handleBoom)))

	// Metrics
	mux.Handle("/metrics", m.MetricsHandler())

	handler := withRequestID(withAccessLog(mux))

	s.httpSrv = &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return s
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return s.httpSrv.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}

// -------- handlers --------

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	slog.Info("hello handler hit", "request_id", requestIDFromCtx(r.Context()))
	fmt.Fprintln(w, "Hello!")
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	// Use cached health status from background checker (non-blocking).
	if s.redis != nil && !s.redis.IsHealthy() {
		slog.Warn("healthz: redis unhealthy", "request_id", requestIDFromCtx(r.Context()))
		http.Error(w, "redis down", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ok")
}

func (s *Server) handleLivez(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ok")
}

func (s *Server) handleBoom(w http.ResponseWriter, r *http.Request) {
	slog.Error("boom endpoint called", "request_id", requestIDFromCtx(r.Context()))
	http.Error(w, "boom", http.StatusInternalServerError)
}

// -------- middleware --------

type ctxKey string

const ctxReqID ctxKey = "req_id"

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-Id")
		if rid == "" {
			rid = newRequestID()
		}
		ctx := context.WithValue(r.Context(), ctxReqID, rid)
		w.Header().Set("X-Request-Id", rid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestIDFromCtx(ctx context.Context) string {
	if v := ctx.Value(ctxReqID); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(p []byte) (int, error) {
	if sw.status == 0 {
		sw.status = http.StatusOK
	}
	n, err := sw.ResponseWriter.Write(p)
	sw.bytes += n
	return n, err
}

func withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}

		next.ServeHTTP(sw, r)

		// Skip noisy probe/metrics paths
		if r.URL.Path == "/livez" || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/metrics" {
			return
		}

		slog.Info("http request",
			"request_id", requestIDFromCtx(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"bytes", sw.bytes,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
