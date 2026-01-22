package main

import (
	"context"
	"fmt"
	"helloapp/src/internal/metrics"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const publicHost = "redis-01.jg88.sat"

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

	// Fail fast on boot (fine for now)
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("failed to connect to cluster: %v", err)
	}
	log.Println("✅ connected to redis cluster")

	m := metrics.New()
	m.SetRedisUp(true)

	// Background redis health + ping duration
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
		}
	}()

	mux := http.NewServeMux()

	// Business endpoint (instrumented)
	mux.Handle("/", m.Middleware("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello!")
	})))

	// Readiness: dependency check (NOT instrumented)
	mux.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := rdb.Ping(c).Err(); err != nil {
			http.Error(w, "redis down", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	}))

	// Liveness: process is up (NOT instrumented)
	mux.Handle("/livez", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	}))

	mux.Handle("/boom", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))

	// Metrics endpoint (NOT instrumented)
	mux.Handle("/metrics", m.MetricsHandler())

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("Server starting on :8080 (metrics on /metrics)")
	log.Fatal(srv.ListenAndServe())
}
