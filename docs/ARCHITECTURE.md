# Architecture

## Package Map

```
cmd/helloapp/main.go
    │
    ├── internal/config      Viper-based config (config.yaml + env override)
    │
    ├── internal/metrics     Prometheus metrics (custom registry)
    │       │
    │       └── Used by: server (HTTP middleware), redis (health gauge)
    │
    ├── internal/redis       Redis client factory + background health checker
    │       │
    │       └── Depends on: metrics (to report redis_up, ping latency)
    │
    └── internal/server      HTTP server, routes, middleware
            │
            └── Depends on: metrics (middleware wrapping), redis (health check)
```

## Dependency Graph

```
config ──────────────────────────────────┐
                                         │
metrics ◄── redis/client ◄── cmd/main ──►│
    ▲                           │        │
    └── server/server ◄─────────┘        │
```

No circular dependencies. `config` and `metrics` are leaf packages.

## Key Flows

### Startup Flow

```
cmd/helloapp/main.go:
  1. config.Load()           → Viper reads config.yaml + env overrides
  2. slog.SetDefault(...)    → configure JSON logger
  3. signal.NotifyContext()   → register SIGINT/SIGTERM
  4. metrics.New()           → create Prometheus registry
  5. redisclient.New(...)    → dial Redis, initial ping
  6. go rc.RunHealthCheck()  → background goroutine: ping every 10s
  7. server.New(...)         → wire routes + middleware
  8. go srv.ListenAndServe() → start HTTP server
  9. <-ctx.Done()            → wait for signal
 10. srv.Shutdown(...)       → drain connections, stop server
 11. rc.Close()              → close Redis
```

### Request Flow

```
HTTP request
  → withRequestID middleware   (inject/propagate X-Request-Id)
  → withAccessLog middleware   (log method, path, status, duration)
  → metrics.Middleware         (count requests, measure latency, in-flight)
  → handler                   (business logic)
  → response
```

### Health Check Flow

```
Background goroutine (every 10s):
  redis.Ping() → update atomic bool + metrics gauge

/healthz request:
  → read atomic bool (non-blocking)
  → 200 OK or 503 Service Unavailable
```

The readiness probe uses the cached health status from the background goroutine rather than performing a synchronous Redis ping on every request. This avoids blocking the probe under load or when Redis is slow.

### Metrics Flow

```
App code → prometheus.Registry (custom, not global)
  │
  ▼
/metrics endpoint → promhttp.HandlerFor(registry)
  │
  ▼
ServiceMonitor (in k8s) → Prometheus scrapes every 30s
  │
  ▼
PrometheusRule → alerts on 5xx rate, P95 latency, Redis down
  │
  ▼
Alertmanager → notifications
```

## Design Decisions

### ADR-001: Viper Config (YAML file + env override)

**Context**: Service needs consistent configuration between local dev and Kubernetes, matching the pattern used by other services (trala).
**Decision**: Use `spf13/viper` to load `config.yaml` with `AutomaticEnv()` override. Structured config with `mapstructure` tags. Env vars use flattened key paths (`SERVER_PORT`, `CACHE_REDIS_PASSWORD`).
**Consequence**: Local dev uses `config.yaml` checked into the repo. Kubernetes overrides specific values via env vars or a mounted Secret. Consistent with trala's `internal/cfg/cfg.go` pattern.

### ADR-002: Custom Prometheus Registry

**Context**: Using the global `prometheus.DefaultRegisterer` makes testing harder and can cause conflicts.
**Decision**: Each `Metrics` instance owns its own `prometheus.Registry`.
**Consequence**: Cleaner testing, no global state, explicit handler via `promhttp.HandlerFor`.

### ADR-003: Non-blocking Readiness Probe

**Context**: Original code called `rdb.Ping()` synchronously on every `/healthz` request.
**Decision**: Background goroutine pings Redis every 10s and stores result in `atomic.Bool`.
**Consequence**: `/healthz` is always fast, no cascading failures when Redis is slow.

### ADR-004: Graceful Shutdown

**Context**: Original code used `srv.ListenAndServe()` with no signal handling — connections could be dropped.
**Decision**: Use `signal.NotifyContext` + `srv.Shutdown(ctx)` with configurable timeout.
**Consequence**: In-flight requests complete before process exits. Clean Redis client close.

### ADR-005: Universal Redis Client

**Context**: Original code used `redis.NewClusterClient` with a `hostAliases` IP rewrite hack.
**Decision**: Switch to `redis.NewUniversalClient` pointing to the in-cluster Predixy gateway.
**Consequence**: No more `hostAliases` hack, works with both cluster and standalone Redis.

