// Command helloapp is the entry point for the Hello World service.
//
// It wires together config, logging, Redis (optional), Audit (optional), metrics, and the HTTP server,
// then runs with graceful shutdown on SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	auditclient "helloapp/internal/audit"
	"helloapp/internal/config"
	"helloapp/internal/metrics"
	redisclient "helloapp/internal/redis"
	"helloapp/internal/server"
)

func main() {
	cfg := config.Load()

	// Setup structured logging
	var logLevel slog.Level
	switch strings.ToUpper(cfg.Server.LoggingLevel) {
	case "DEBUG":
		logLevel = slog.LevelDebug
	case "WARN", "WARNING":
		logLevel = slog.LevelWarn
	case "ERROR":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})).With("service", cfg.Server.ServiceName)
	slog.SetDefault(logger)

	slog.Info("starting service",
		"port", cfg.Server.Port,
		"redis_enabled", cfg.Cache.Enabled,
		"audit_enabled", cfg.Audit.Enabled,
		"log_level", cfg.Server.LoggingLevel,
	)

	// Context for startup + shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Initialize metrics
	m := metrics.New()

	// Parse durations from config strings
	redisTimeout := parseDuration(cfg.Cache.RedisTimeout, 5*time.Second)
	shutdownTimeout := parseDuration(cfg.Server.ShutdownTimeout, 15*time.Second)

	// Initialize Redis (optional)
	var rc *redisclient.Client
	if cfg.Cache.Enabled && len(cfg.Cache.RedisAddrs) > 0 {
		connectCtx, connectCancel := context.WithTimeout(ctx, 30*time.Second)
		defer connectCancel()

		var err error
		rc, err = redisclient.New(connectCtx, cfg.Cache.RedisAddrs, cfg.Cache.RedisPassword, redisTimeout, m)
		if err != nil {
			slog.Error("failed to connect to redis", "err", err, "addrs", cfg.Cache.RedisAddrs)
			os.Exit(1)
		}
		defer func() { _ = rc.Close() }()
		slog.Info("connected to redis")
		go rc.RunHealthCheck(ctx, 10*time.Second)
	} else {
		slog.Info("redis disabled")
		m.SetRedisUp(false) // Mark as disabled, not down
	}

	// Initialize Audit client (optional)
	var ac *auditclient.Client
	if cfg.Audit.Enabled && cfg.Audit.GRPCUrl != "" {
		connectCtx, connectCancel := context.WithTimeout(ctx, 10*time.Second)
		defer connectCancel()

		var err error
		ac, err = auditclient.New(connectCtx, cfg.Audit.GRPCUrl)
		if err != nil {
			slog.Warn("failed to connect to audit service", "err", err, "url", cfg.Audit.GRPCUrl)
			// Non-fatal — continue without audit
		} else if ac != nil {
			defer func() { _ = ac.Close() }()
		}
	} else {
		slog.Info("audit disabled")
	}

	// Create and start HTTP server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := server.New(addr, m, rc, ac)

	// Run server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		slog.Info("server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for shutdown signal or server error
	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			slog.Error("server error", "err", err)
		}
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	slog.Info("shutting down server", "timeout", shutdownTimeout)
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "err", err)
		os.Exit(1)
	}

	slog.Info("server stopped gracefully")
}

// parseDuration parses a duration string, returning fallback on error.
func parseDuration(s string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}
