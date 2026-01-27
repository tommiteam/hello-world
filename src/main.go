package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"helloapp/src/internal/metrics"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const publicHost = "redis-01.jg88.sat"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rdb := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs: []string{publicHost + ":7000"},
		NewClient: func(opt *redis.Options) *redis.Client {
			host, port, err := net.SplitHostPort(opt.Addr)
			if err == nil && strings.HasPrefix(host, "172.16.") {
				opt.Addr = net.JoinHostPort(publicHost, port)
			}
			return redis.NewClient(opt)
		},
		DialTimeout:  10 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	})
	defer func() { _ = rdb.Close() }()

	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Error("failed to connect to redis cluster", "err", err)
		os.Exit(1)
	}
	slog.Info("connected to redis cluster")

	m := metrics.New()
	m.SetRedisUp(true)

	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for range t.C {
			start := time.Now()
			c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err := rdb.Ping(c).Err()
			cancel()

			m.ObserveRedisPing(time.Since(start).Seconds())
			m.SetRedisUp(err == nil)

			if err != nil {
				slog.Warn("redis ping failed", "err", err)
			} else {
				slog.Info("redis ping ok", "latency_ms", time.Since(start).Milliseconds())
			}
		}
	}()

	mux := http.NewServeMux()

	// Business endpoint
	mux.Handle("/", m.Middleware("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("hello handler hit", "request_id", requestIDFromCtx(r.Context()))
		fmt.Fprintln(w, "Hello!")
	})))

	mux.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := rdb.Ping(c).Err(); err != nil {
			slog.Warn("healthz redis down", "err", err, "request_id", requestIDFromCtx(r.Context()))
			http.Error(w, "redis down", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	}))

	mux.Handle("/livez", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	}))

	mux.Handle("/boom", m.Middleware("/boom", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Error("boom endpoint called", "request_id", requestIDFromCtx(r.Context()))
		http.Error(w, "boom", http.StatusInternalServerError)
	})))

	mux.Handle("/metrics", m.MetricsHandler())

	// Wrap whole mux with access log middleware
	handler := withRequestID(withAccessLog(mux))

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("server starting", "addr", ":8080")
	slog.Error("server stopped", "err", srv.ListenAndServe())
}

// -------- middleware helpers --------

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
