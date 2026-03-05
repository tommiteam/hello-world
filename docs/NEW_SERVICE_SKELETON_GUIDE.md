# New Service Skeleton Guide

Use the `hello-world` project as a template to create a new Go HTTP service. This guide is designed to be used by both humans and AI coding assistants.

---

## Required Structure

```
<service-name>/
├── cmd/
│   └── <service-name>/
│       └── main.go             # Thin entrypoint: config → deps → server → shutdown
├── internal/
│   ├── config/
│   │   ├── config.go           # Viper-based config (config.yaml + env override)
│   │   └── config_test.go
│   ├── metrics/
│   │   └── metrics.go          # Custom Prometheus registry + middleware
│   ├── redis/                  # (optional) Redis client + health checker
│   │   └── client.go
│   └── server/
│       ├── server.go           # HTTP server, routes, all middleware
│       └── server_test.go
├── config.yaml                 # Local dev only (gitignored — mounted via K8s Secret in prod)
├── docs/
│   ├── ARCHITECTURE.md
│   └── OPERABILITY.md
├── Dockerfile                  # Multi-stage: golang builder → distroless
├── go.mod
├── go.sum
└── README.md
```

## Naming Conventions

| Item | Convention | Example |
|------|-----------|---------|
| Module name | Short, lowercase | `module myservice` |
| Binary name | Same as module | `cmd/myservice/main.go` |
| Package names | Single lowercase word | `config`, `server`, `metrics` |
| Config env vars | `SECTION_KEY` (flattened YAML path) | `SERVER_PORT`, `CACHE_REDIS_PASSWORD` |
| Metric names | `<service>_<subsystem>_<metric>` | `myservice_http_requests_total` |
| Docker image | `ghcr.io/tommiteam/<service-name>` | `ghcr.io/tommiteam/myservice` |

## Step-by-Step: Create a New Service

### 1. Initialize the Go module

```bash
mkdir <service-name> && cd <service-name>
go mod init <service-name>
mkdir -p cmd/<service-name> internal/{config,metrics,server} docs
```

### 2. Copy and adapt config (Viper + config.yaml)

From `hello-world/internal/config/config.go`. This uses [Viper](https://github.com/spf13/viper) to:
1. Read `config.yaml` from the working directory
2. Apply env var overrides via `AutomaticEnv()` (flattened key path: `SERVER_PORT`, `CACHE_REDIS_PASSWORD`)
3. Unmarshal into typed structs with `mapstructure` tags

**Modify:**
- Add/remove struct fields + `mapstructure` tags for your needs
- Update `viper.SetDefault(...)` calls to match
- Create a `config.yaml` with dev-friendly defaults

```go
type Config struct {
    Server   Server   `mapstructure:"server"`
    Database Database `mapstructure:"database"`
    Cache    Cache    `mapstructure:"cache"`
}

type Server struct {
    ServiceName     string `mapstructure:"service_name"`
    Port            int    `mapstructure:"port"`
    LoggingLevel    string `mapstructure:"logging_level"`
    ShutdownTimeout string `mapstructure:"shutdown_timeout"`
}

type Database struct {
    PostgresURL string `mapstructure:"postgres_url"`
}

type Cache struct {
    RedisAddrs    []string `mapstructure:"redis_addrs"`
    RedisPassword string   `mapstructure:"redis_password"`
    RedisTimeout  string   `mapstructure:"redis_timeout"`
}
```

And create `config.yaml`:
```yaml
server:
  service_name: myservice
  port: 8080
  logging_level: INFO
  shutdown_timeout: 15s
database:
  postgres_url: ""
cache:
  redis_addrs:
    - localhost:6379
  redis_password: ""
  redis_timeout: 5s
```

### 3. Copy `internal/metrics/metrics.go`

From `hello-world/internal/metrics/metrics.go`. Modify:
- Rename metric prefixes: `hello_` → `<service>_`
- Remove `redisUp`/`redisPingDur` if not using Redis
- Add domain-specific metrics as needed

Key pattern:
```go
func New() *Metrics {
    reg := prometheus.NewRegistry()
    // Register Go + process collectors
    // Register custom metrics
    return &Metrics{Registry: reg, ...}
}
```

### 4. Create `internal/server/server.go`

From `hello-world/internal/server/server.go`. This contains:
- Route registration in `New()`
- Handler methods (`handleRoot`, `handleHealthz`, `handleLivez`)
- Middleware chain: `withRequestID` → `withAccessLog` → `metrics.Middleware` → handler
- The `Server` struct wrapping `http.Server`

**To add a new HTTP route:**

```go
func New(addr string, m *metrics.Metrics, /* deps */) *Server {
    s := &Server{/* deps */}
    mux := http.NewServeMux()

    // Existing routes...
    mux.Handle("/", m.Middleware("/", http.HandlerFunc(s.handleRoot)))

    // ADD NEW ROUTE HERE:
    mux.Handle("/api/v1/widgets", m.Middleware("/api/v1/widgets", http.HandlerFunc(s.handleWidgets)))

    // Health + metrics routes (always present)
    mux.Handle("/healthz", http.HandlerFunc(s.handleHealthz))
    mux.Handle("/livez", http.HandlerFunc(s.handleLivez))
    mux.Handle("/metrics", m.MetricsHandler())

    // Wrap with middleware
    handler := withRequestID(withAccessLog(mux))
    // ...
}

func (s *Server) handleWidgets(w http.ResponseWriter, r *http.Request) {
    // Your business logic
}
```

### 5. Create `cmd/<service-name>/main.go`

From `hello-world/cmd/helloapp/main.go`. The pattern:

```go
func main() {
    cfg := config.Load()
    // 1. Setup logger
    // 2. signal.NotifyContext (SIGINT, SIGTERM)
    // 3. Initialize dependencies (Redis, DB, etc.)
    // 4. Create server
    // 5. go srv.ListenAndServe()
    // 6. <-ctx.Done()  ← wait for signal
    // 7. srv.Shutdown(shutdownCtx)
    // 8. Close dependencies
}
```

### 6. Add dependencies (DB, Redis, external clients)

Create a new package under `internal/`:

```
internal/
  └── postgres/
      └── client.go
```

**Pattern for a dependency client:**

```go
package postgres

type Client struct {
    db      *sql.DB
    healthy atomic.Bool
}

func New(ctx context.Context, dsn string) (*Client, error) {
    db, err := sql.Open("postgres", dsn)
    if err != nil { return nil, err }
    if err := db.PingContext(ctx); err != nil { return nil, err }
    c := &Client{db: db}
    c.healthy.Store(true)
    return c, nil
}

func (c *Client) IsHealthy() bool { return c.healthy.Load() }
func (c *Client) Close() error    { return c.db.Close() }

func (c *Client) RunHealthCheck(ctx context.Context, interval time.Duration) {
    // Same pattern as redis/client.go
}
```

**Wire it in `main.go`:**

```go
pgClient, err := postgres.New(ctx, cfg.DatabaseURL)
if err != nil { ... }
defer pgClient.Close()
go pgClient.RunHealthCheck(ctx, 10*time.Second)

srv := server.New(addr, m, pgClient)
```

**Wire it in health check:**

```go
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
    if s.pg != nil && !s.pg.IsHealthy() {
        http.Error(w, "db down", http.StatusServiceUnavailable)
        return
    }
    // ...
}
```

### 7. Interfaces and Mocks (for testing)

Define interfaces at the **consumer** side (in `server` package):

```go
// internal/server/deps.go
type RedisChecker interface {
    IsHealthy() bool
}

type WidgetStore interface {
    List(ctx context.Context) ([]Widget, error)
    Create(ctx context.Context, w Widget) error
}
```

Then `Server` accepts interfaces:

```go
type Server struct {
    redis  RedisChecker
    store  WidgetStore
    // ...
}
```

For tests, create mock implementations:

```go
type mockStore struct {
    widgets []Widget
    err     error
}

func (m *mockStore) List(ctx context.Context) ([]Widget, error) {
    return m.widgets, m.err
}
```

### 8. Wire telemetry

#### Metrics

Already handled by the `metrics.Middleware` wrapper. For custom business metrics:

```go
// In metrics.go
widgetsCreated := prometheus.NewCounter(prometheus.CounterOpts{
    Name: "myservice_widgets_created_total",
    Help: "Total widgets created.",
})
reg.MustRegister(widgetsCreated)
```

#### Traces (OpenTelemetry → Jaeger)

```bash
go get go.opentelemetry.io/otel
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc
go get go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
```

In `main.go`:
```go
import "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"

exporter, _ := otlptracegrpc.New(ctx,
    otlptracegrpc.WithEndpoint("jaeger-collector.monitoring.svc.cluster.local:4317"),
    otlptracegrpc.WithInsecure(),
)
tp := trace.NewTracerProvider(trace.WithBatcher(exporter))
otel.SetTracerProvider(tp)
defer tp.Shutdown(ctx)
```

Wrap the HTTP mux with `otelhttp`:
```go
handler := otelhttp.NewHandler(withRequestID(withAccessLog(mux)), cfg.Server.ServiceName)
```

#### Log Correlation

Add trace ID to logs:
```go
span := trace.SpanFromContext(r.Context())
slog.Info("handling request",
    "trace_id", span.SpanContext().TraceID().String(),
    "request_id", requestIDFromCtx(r.Context()),
)
```

### 9. Dockerfile

```dockerfile
FROM golang:1.24.2 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=linux go build -o /app-binary ./cmd/<service-name>

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=builder /app-binary /app-binary
EXPOSE 8080
ENTRYPOINT ["/app-binary"]
```

> **Do NOT copy config.yaml into the image.** It contains secrets.
> In Kubernetes, config.yaml is mounted via a SealedSecret volume (see step 10).

### 10. Kubernetes manifests

Follow the [ADD_NEW_SERVICE_GUIDE.md](/infra/docs/ADD_NEW_SERVICE_GUIDE.md) in the infra repo to create:
- `deployment.yaml` (with health probes pointing to `/livez` and `/healthz`)
- `service.yaml`
- `ingress.yaml`
- `kustomization.yaml`
- `servicemonitor.yaml` (optional)
- `prometheusrule.yaml` (optional)

---

## Checklists

### Before Shipping

- [ ] `go build ./cmd/<service-name>` succeeds
- [ ] `go test ./...` passes
- [ ] `go vet ./...` clean
- [ ] All config via `config.yaml` + env override (Viper, no hardcoded values)
- [ ] Graceful shutdown implemented (signal handling + `srv.Shutdown`)
- [ ] `/livez` endpoint returns 200
- [ ] `/healthz` endpoint checks all critical dependencies
- [ ] `/metrics` endpoint exposes Prometheus metrics
- [ ] Structured JSON logging on stdout
- [ ] Request-ID propagation (X-Request-Id header)
- [ ] Dockerfile builds successfully
- [ ] README.md documents endpoints, config, and how to run

### Before Deploying to Cluster

- [ ] Docker image pushed to GHCR
- [ ] K8s manifests created in infra repo (`cluster/apps/<name>/`)
- [ ] Argo CD Application config created (`cluster/apps-config/<name>.yaml`)
- [ ] Secrets sealed (if any)
- [ ] ServiceMonitor created (if metrics exposed)
- [ ] PrometheusRule created (if alerts needed)
- [ ] DNS / tunnel routing verified
- [ ] Health probes configured in Deployment
- [ ] Resource requests/limits set

### Ongoing Operations

- [ ] Alerts firing correctly (test with `/boom`)
- [ ] Logs visible in Grafana/Loki
- [ ] Metrics visible in Grafana/Prometheus
- [ ] Graceful shutdown works (test with `kubectl rollout restart`)

