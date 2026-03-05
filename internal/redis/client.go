// Package redisclient provides a Redis client factory and health-check goroutine.
package redisclient

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"helloapp/internal/metrics"

	"github.com/redis/go-redis/v9"
)

// Client wraps a go-redis UniversalClient with a background health checker.
type Client struct {
	rdb     redis.UniversalClient
	healthy atomic.Bool
	metrics *metrics.Metrics
}

// New creates and pings a Redis client. It returns an error if the initial ping fails.
// The caller should call Close() to release resources and Stop() to halt the health goroutine.
func New(ctx context.Context, addrs []string, password string, timeout time.Duration, m *metrics.Metrics) (*Client, error) {
	rdb := redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs:        addrs,
		Password:     password,
		DialTimeout:  timeout,
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, err
	}

	c := &Client{
		rdb:     rdb,
		metrics: m,
	}
	c.healthy.Store(true)
	m.SetRedisUp(true)

	return c, nil
}

// Underlying returns the raw go-redis UniversalClient for direct use.
func (c *Client) Underlying() redis.UniversalClient {
	return c.rdb
}

// IsHealthy returns the cached health status (updated by the background checker).
func (c *Client) IsHealthy() bool {
	return c.healthy.Load()
}

// Close closes the underlying Redis connection.
func (c *Client) Close() error {
	return c.rdb.Close()
}

// RunHealthCheck starts a blocking loop that pings Redis every interval.
// It updates the health status and metrics. Exits when ctx is cancelled.
func (c *Client) RunHealthCheck(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			start := time.Now()
			pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			err := c.rdb.Ping(pingCtx).Err()
			cancel()

			elapsed := time.Since(start)
			c.metrics.ObserveRedisPing(elapsed.Seconds())

			if err != nil {
				c.healthy.Store(false)
				c.metrics.SetRedisUp(false)
				slog.Warn("redis ping failed", "err", err, "latency_ms", elapsed.Milliseconds())
			} else {
				c.healthy.Store(true)
				c.metrics.SetRedisUp(true)
				slog.Debug("redis ping ok", "latency_ms", elapsed.Milliseconds())
			}
		}
	}
}
