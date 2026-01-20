package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

const publicHost = "redis-01.jg88.sat"

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hello_http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"path", "method", "code"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hello_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"path", "method"},
	)

	redisUp = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "hello_redis_up",
		Help: "Whether Redis cluster is reachable (1 = up, 0 = down).",
	})
)

func init() {
	prometheus.MustRegister(httpRequestsTotal, httpRequestDuration, redisUp)
}

func main() {
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

	// Initial ping (fail fast)
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("failed to connect to cluster: %v", err)
	}
	log.Println("✅ connected to redis cluster")
	redisUp.Set(1)

	// Background health check
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for range t.C {
			c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err := rdb.Ping(c).Err()
			cancel()
			if err != nil {
				redisUp.Set(0)
				continue
			}
			redisUp.Set(1)
		}
	}()

	mux := http.NewServeMux()

	// Your existing endpoint, with simple instrumentation
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		code := 200
		defer func() {
			httpRequestsTotal.WithLabelValues("/", r.Method, fmt.Sprint(code)).Inc()
			httpRequestDuration.WithLabelValues("/", r.Method).Observe(time.Since(start).Seconds())
		}()

		fmt.Fprintln(w, "Hello!")
	})

	// Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("Server starting on :8080 (metrics on /metrics)")
	log.Fatal(srv.ListenAndServe())
}
