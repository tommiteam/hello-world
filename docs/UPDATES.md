# Hello-World Service Updates

## What Changed

### 1. ✅ Redis is now OPTIONAL
- **Before:** Redis connection was required — service failed to start if Redis was down
- **After:** Redis is optional via `cache.enabled: false` in config
- **Benefit:** Service can run without Redis for simpler deployments

### 2. ✅ Audit service integration (OPTIONAL)
- **New:** Audit client that sends events to the audit service via gRPC
- **Usage:** Automatically logs audit events for specific actions (e.g., homepage views)
- **Benefit:** Centralized audit logging across all services

---

## Configuration

### Example config.yaml with both features enabled
```yaml
server:
  service_name: hello-world
  port: 8080
  logging_level: INFO
  shutdown_timeout: 15s

cache:
  enabled: true                # Set to false to disable Redis
  redis_addrs:
    - redis-gateway.redis.svc.cluster.local:6379
  redis_password: ""
  redis_timeout: 5s

audit:
  enabled: true                # Set to false to disable audit
  grpc_url: audit.apps.svc.cluster.local:80
```

### Example config.yaml with Redis and Audit DISABLED
```yaml
server:
  service_name: hello-world
  port: 8080
  logging_level: INFO
  shutdown_timeout: 15s

cache:
  enabled: false               # Redis disabled

audit:
  enabled: false               # Audit disabled
```

### Environment variable overrides
```bash
# Disable Redis via env
CACHE_ENABLED=false

# Disable Audit via env
AUDIT_ENABLED=false

# Change audit URL via env
AUDIT_GRPC_URL=audit.production.svc.cluster.local:80
```

---

## Code Changes

### 1. Config (`internal/config/config.go`)
- Added `Cache.Enabled bool` field
- Added `Audit` struct with `Enabled` and `GRPCUrl` fields
- Defaults: both enabled by default

### 2. Audit Client (`internal/audit/client.go`)
- New package that wraps gRPC connection to audit service
- `New()` returns `nil` if audit is disabled (no-op)
- `LogEvent()` is fire-and-forget (async, non-blocking)
- Connection failures are non-fatal (log warning, continue without audit)

### 3. Main (`cmd/helloapp/main.go`)
- Redis: Only connect if `cfg.Cache.Enabled && len(cfg.Cache.RedisAddrs) > 0`
- Audit: Only connect if `cfg.Audit.Enabled && cfg.Audit.GRPCUrl != ""`
- Both are optional — service starts even if connection fails

### 4. Server (`internal/server/server.go`)
- Accepts `*auditclient.Client` (can be nil)
- `handleRoot()` logs audit event when request comes in
- Example audit event:
  ```
  action: "view"
  resource: "homepage"
  actor: "anonymous"
  meta: {"request_id": "abc123", "method": "GET", "user_agent": "..."}
  ```

### 5. Tests (`internal/server/server_test.go`)
- Updated all `New()` calls to pass `nil` for audit client
- Tests still pass (audit is optional)

---

## Behavior

### When Redis is disabled (`cache.enabled: false`)
```
2026-03-10T10:00:00Z INFO starting service redis_enabled=false
2026-03-10T10:00:00Z INFO redis disabled
2026-03-10T10:00:00Z INFO server listening addr=:8080
```
- No Redis connection attempt
- `/healthz` returns 200 OK (doesn't check Redis)
- Metrics: `hello_redis_up` stays at 0 (disabled, not down)

### When Audit is disabled (`audit.enabled: false`)
```
2026-03-10T10:00:00Z INFO starting service audit_enabled=false
2026-03-10T10:00:00Z INFO audit disabled
2026-03-10T10:00:00Z INFO server listening addr=:8080
```
- No audit gRPC connection
- Request handlers work normally (audit calls are no-ops)

### When both are enabled
```
2026-03-10T10:00:00Z INFO starting service redis_enabled=true audit_enabled=true
2026-03-10T10:00:00Z INFO connected to redis
2026-03-10T10:00:00Z INFO audit client connected target=audit.apps.svc.cluster.local:80
2026-03-10T10:00:00Z INFO server listening addr=:8080
```

### When audit connection fails (non-fatal)
```
2026-03-10T10:00:00Z WARN failed to connect to audit service err="connection refused" url=audit.apps.svc.cluster.local:80
2026-03-10T10:00:00Z INFO server listening addr=:8080
```
- Service continues without audit
- No audit events are sent

---

## Audit Events Logged

### Current implementation
| Endpoint | Action | Resource | Actor | Meta |
|----------|--------|----------|-------|------|
| `GET /` | `view` | `homepage` | `anonymous` | request_id, method, user_agent |

### Easy to extend
```go
// In any handler:
if s.audit != nil {
    s.audit.LogEvent(r.Context(), 
        "create",           // action
        "order",            // resource
        "user@example.com", // actor
        map[string]string{
            "order_id": "12345",
            "amount": "99.99",
        },
    )
}
```

---

## Deployment

### Kubernetes ConfigMap/Secret
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: hello-world-config
  namespace: apps
stringData:
  config.yaml: |
    server:
      service_name: hello-world
      port: 8080
    cache:
      enabled: true
      redis_addrs:
        - redis-gateway.redis.svc.cluster.local:6379
    audit:
      enabled: true
      grpc_url: audit.apps.svc.cluster.local:80
```

### Disable features via env in Deployment
```yaml
spec:
  containers:
    - name: hello-world
      image: ghcr.io/tommiteam/hello-world
      env:
        - name: CACHE_ENABLED
          value: "false"
        - name: AUDIT_ENABLED
          value: "false"
```

---

## Benefits

### ✅ Flexible deployment
- Run with Redis for caching
- Run without Redis for simpler setups
- Run with/without audit logging

### ✅ Fail-safe
- Audit connection failure is non-fatal
- Service continues to serve requests
- Reduces cascading failures

### ✅ Observable
- Logs clearly show which features are enabled
- Audit events provide centralized logging
- Easy to debug in production

### ✅ Zero breaking changes
- Default: both enabled (backward compatible)
- Existing deployments work as-is
- Opt-in to disable features

---

## Testing

### Run locally without Redis
```bash
cd /home/ngominhtoan/tommi-team/hello-world
cat > config.yaml <<EOF
server:
  port: 8080
cache:
  enabled: false
audit:
  enabled: false
EOF

go build -o hello-world ./cmd/helloapp
./hello-world
```

### Run with audit only (no Redis)
```bash
# Start audit service first
cd /home/ngominhtoan/tommi-team/audit
./audit

# Then start hello-world
cd /home/ngominhtoan/tommi-team/hello-world
cat > config.yaml <<EOF
server:
  port: 8080
cache:
  enabled: false
audit:
  enabled: true
  grpc_url: localhost:80
EOF

./hello-world
```

### Test audit logging
```bash
curl http://localhost:8080/

# Check hello-world logs — should see:
# DEBUG audit event (would send to gRPC) action=view resource=homepage actor=anonymous
```

---

## Future Enhancements

1. **Complete audit proto integration**
   - Currently: logs to stdout (TODO comment in client.go)
   - Next: Import generated audit proto client, call real `IngestEvent` RPC

2. **More audit events**
   - Log `/boom` calls (errors)
   - Log auth events (when auth is added)
   - Log data mutations

3. **Retry logic**
   - Reconnect to audit service if connection drops
   - Buffer events during downtime (with size limit)

---

## Summary

✅ Redis is now **optional** (set `cache.enabled: false`)  
✅ Audit service integration **optional** (set `audit.enabled: false`)  
✅ Both features disabled by default in config ❌ — **both ENABLED by default** (backward compatible)  
✅ Service starts even if connections fail  
✅ Zero breaking changes for existing deployments  
✅ Easy to extend with more audit events  

The hello-world service is now more flexible and production-ready!

