# Operability Guide

How this service is observed, configured, and operated in the k3s cluster.

## Logging

- **Library**: `log/slog` (Go stdlib, structured JSON)
- **Output**: stdout (collected by Alloy DaemonSet → Loki)
- **Level**: Configurable via `LOG_LEVEL` env var (default: `INFO`)
- **Fields**: Every log line includes `service` name; HTTP requests include `request_id`, `method`, `path`, `status`, `duration_ms`
- **Probe/metrics paths** (`/livez`, `/healthz`, `/readyz`, `/metrics`) are excluded from access logs to reduce noise

### Log Query (Grafana → Loki)

```logql
{namespace="apps", app="hello-world"} | json | level="ERROR"
```

## Metrics

**Endpoint**: `/metrics` (Prometheus exposition format)

### Custom Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `hello_http_in_flight_requests` | Gauge | — | Currently processing requests |
| `hello_http_requests_total` | Counter | route, method, code | Total HTTP requests |
| `hello_http_request_duration_seconds` | Histogram | route, method | Request latency |
| `hello_redis_up` | Gauge | — | Redis reachable (1=up, 0=down) |
| `hello_redis_ping_duration_seconds` | Histogram | — | Redis ping latency |

### Go Runtime Metrics

Standard Go collector and process collector are registered (goroutines, GC, memory, file descriptors).

### Prometheus Scraping

Defined in `/infra/cluster/apps/hello-world/servicemonitor.yaml`:
```yaml
endpoints:
  - port: http
    path: /metrics
    interval: 30s
```

The ServiceMonitor requires `labels.release: monitoring` to be picked up by kube-prometheus-stack.

## Alerts

Defined in `/infra/cluster/apps/hello-world/prometheusrule.yaml`:

| Alert | Expression | For | Severity |
|-------|-----------|-----|----------|
| HelloWorldHigh5xxRate | 5xx rate > 5% | 2m | warning |
| HelloWorldHighP95Latency | P95 > 250ms | 2m | warning |
| HelloWorldRedisDown | `hello_redis_up == 0` | 1m | critical |

## Tracing

**Current state**: Not yet instrumented with OpenTelemetry.

**Integration path** (when ready):
- Jaeger collector is at `jaeger-collector.monitoring.svc.cluster.local:4317` (OTLP gRPC)
- Add `go.opentelemetry.io/otel` + `otlptracegrpc` exporter
- Wrap HTTP handlers with `otelhttp`
- Pass trace context via `X-Request-Id` or W3C traceparent header

## Health Probes

### Kubernetes Configuration

From `/infra/cluster/apps/hello-world/deployment.yaml`:

```yaml
startupProbe:
  httpGet:
    path: /livez
    port: http
  periodSeconds: 2
  failureThreshold: 30    # 60s startup budget

livenessProbe:
  httpGet:
    path: /livez
    port: http
  periodSeconds: 10
  timeoutSeconds: 2
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /healthz
    port: http
  periodSeconds: 10
  timeoutSeconds: 2
  failureThreshold: 3
```

### Endpoint Behavior

| Endpoint | Checks | Failure Mode |
|----------|--------|-------------|
| `/livez` | Process alive | Always 200 (unless process crashed) |
| `/healthz` | Redis health (cached) | 503 if Redis is down |
| `/readyz` | Alias for `/healthz` | Same as above |

The readiness check uses a **cached** Redis health status updated by a background goroutine every 10 seconds. It never blocks on a live Redis ping.

## Configuration

Configuration is loaded by [Viper](https://github.com/spf13/viper) from `config.yaml` (mounted at `/app/config.yaml` via a Kubernetes Secret) and can be overridden per-key by environment variables.

See [README.md](../README.md#configuration) for the full config reference.

Key operational overrides (set via env or by editing the Secret's `config.yaml`):
- `SERVER_LOGGING_LEVEL=DEBUG` — enable verbose logging for troubleshooting
- `SERVER_SHUTDOWN_TIMEOUT=30s` — increase if long-lived connections exist
- `CACHE_REDIS_TIMEOUT=10s` — increase if Redis is slow

In Kubernetes, the config.yaml is delivered via a SealedSecret:
```
infra/cluster/apps/hello-world/secret.yaml      ← plaintext (never committed)
    → make secret-hello-world
infra/cluster/apps/hello-world/secret.sealed.yaml ← committed, synced by Argo CD
    → K8s Secret "hello-world-config"
    → mounted at /app/config.yaml in the pod
```

## Deployment Expectations

### Resource Requirements

Currently no resource limits are set in the Deployment. Recommended:
```yaml
resources:
  requests:
    cpu: 50m
    memory: 64Mi
  limits:
    cpu: 200m
    memory: 256Mi
```

### Scaling

- Default: 1 replica
- Stateless — safe to scale horizontally
- No leader election or singleton requirements

### Dependencies

| Dependency | Type | Required | Failure Impact |
|-----------|------|----------|---------------|
| Redis (via Predixy gateway) | Data store | Yes (for readiness) | `/healthz` returns 503, pod marked unready |

### Image Pull

Images are pulled from `ghcr.io/tommiteam/hello-world` using the `ghcr-cred-v2` image pull secret.

### Rollback

Image tag is managed via Kustomize `images.newTag` in `/infra/cluster/apps/hello-world/kustomization.yaml`. To rollback:
```bash
cd /path/to/infra
# Edit cluster/apps/hello-world/kustomization.yaml → change newTag to previous SHA
git commit -am "rollback hello-world to <sha>"
git push
```

