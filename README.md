# hello-world

A clean, production-ready Go HTTP service skeleton deployed to a k3s homelab cluster via Argo CD.

## Features

- Structured JSON logging (`log/slog`)
- Prometheus metrics (`/metrics`)
- Redis connectivity with background health checker
- Graceful shutdown (SIGINT/SIGTERM)
- Request-ID propagation (X-Request-Id header)
- Health/readiness/liveness probes
- Viper-based configuration (`config.yaml` + env override)

## Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Returns `Hello!` |
| `/healthz` | GET | Readiness probe — returns 503 if Redis is unhealthy |
| `/readyz` | GET | Alias for `/healthz` |
| `/livez` | GET | Liveness probe — always returns 200 |
| `/boom` | GET | Test endpoint — returns 500 |
| `/metrics` | GET | Prometheus metrics |

## Configuration

Configuration is loaded from `config.yaml` (via [Viper](https://github.com/spf13/viper)) and can be overridden by environment variables.

### config.yaml

```yaml
server:
  service_name: hello-world
  port: 8080
  logging_level: INFO        # DEBUG | INFO | WARN | ERROR
  shutdown_timeout: 15s

cache:
  redis_addrs:
    - redis-gateway.redis.svc.cluster.local:6379
  redis_password: ""
  redis_timeout: 5s
```

### Environment Variable Override

Every YAML key can be overridden by an env var using the flattened path with `_` separators:

| Env Variable | YAML Key | Default |
|-------------|----------|---------|
| `SERVER_SERVICE_NAME` | `server.service_name` | `hello-world` |
| `SERVER_PORT` | `server.port` | `8080` |
| `SERVER_LOGGING_LEVEL` | `server.logging_level` | `INFO` |
| `SERVER_SHUTDOWN_TIMEOUT` | `server.shutdown_timeout` | `15s` |
| `CACHE_REDIS_ADDRS` | `cache.redis_addrs` | `redis-gateway.redis.svc.cluster.local:6379` |
| `CACHE_REDIS_PASSWORD` | `cache.redis_password` | (empty) |
| `CACHE_REDIS_TIMEOUT` | `cache.redis_timeout` | `5s` |

Priority: **env vars > config.yaml > built-in defaults**

## Run Locally

### Prerequisites

- Go 1.24+
- Redis (optional — service starts but readiness fails without it)

### Without Redis

```bash
go run ./cmd/helloapp
# Server starts on :8080 (reads config.yaml from current directory)
# /livez returns 200, /healthz returns 503 (no redis)
```

### With Redis

```bash
# Start a local Redis
docker run -d -p 6379:6379 redis:7

# Option A: edit config.yaml
#   cache.redis_addrs: ["localhost:6379"]

# Option B: env override
CACHE_REDIS_ADDRS=localhost:6379 go run ./cmd/helloapp
```

## Build

### Binary

```bash
go build -o hello-app ./cmd/helloapp
```

### Docker

```bash
docker build -t hello-world .
docker run -p 8080:8080 -e CACHE_REDIS_ADDRS=host.docker.internal:6379 hello-world
```

## Test

```bash
go test ./...
go vet ./...
```

## Project Structure

```
hello-world/
├── cmd/
│   └── helloapp/
│       └── main.go             # Entry point: config, wiring, graceful shutdown
├── internal/
│   ├── config/
│   │   ├── config.go           # Viper-based config (config.yaml + env override)
│   │   └── config_test.go
│   ├── metrics/
│   │   └── metrics.go          # Prometheus metrics (custom registry)
│   ├── redis/
│   │   └── client.go           # Redis client factory + background health check
│   └── server/
│       ├── server.go           # HTTP server, routes, middleware
│       └── server_test.go
├── config.yaml                 # Default configuration (baked into Docker image)
├── docs/
│   ├── ARCHITECTURE.md         # Package map + key flows
│   ├── OPERABILITY.md          # Logging, metrics, tracing, probes
│   └── NEW_SERVICE_SKELETON_GUIDE.md  # How to create a new service from this skeleton
├── Dockerfile                  # Multi-stage build (golang → distroless)
├── go.mod
└── go.sum
```

### Legacy (to be removed)

The `src/` directory contains the original monolithic code. It is superseded by `cmd/` + `internal/` and can be deleted once the migration is verified in production.

## Deployment

This service is deployed to k3s via Argo CD. The infra repo at `cluster/apps/hello-world/` contains:
- `deployment.yaml` — Kubernetes Deployment with health probes
- `service.yaml` — ClusterIP Service
- `ingress.yaml` — Ingress at `hello-world.tommitoan.space`
- `servicemonitor.yaml` — Prometheus scraping
- `prometheusrule.yaml` — Alerts (5xx rate, P95 latency, Redis down)
- `kustomization.yaml` — Image tag managed by CI

CI pushes a new image tag → patches `kustomization.yaml` → Argo CD syncs automatically.

