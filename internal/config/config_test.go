package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear env vars that might interfere
	for _, key := range []string{
		"SERVER_PORT", "SERVER_SERVICE_NAME", "SERVER_LOGGING_LEVEL",
		"SERVER_SHUTDOWN_TIMEOUT", "CACHE_ENABLED", "CACHE_REDIS_ADDRS",
		"CACHE_REDIS_PASSWORD", "CACHE_REDIS_TIMEOUT",
		"AUDIT_ENABLED", "AUDIT_GRPC_URL",
	} {
		os.Unsetenv(key)
	}

	// Load from a directory with no config.yaml → pure defaults
	cfg := Load(t.TempDir())

	if cfg.Server.Port != 8080 {
		t.Errorf("expected Port=8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.ServiceName != "hello-world" {
		t.Errorf("expected ServiceName=hello-world, got %s", cfg.Server.ServiceName)
	}
	if cfg.Server.LoggingLevel != "INFO" {
		t.Errorf("expected LoggingLevel=INFO, got %s", cfg.Server.LoggingLevel)
	}
	if cfg.Server.ShutdownTimeout != "15s" {
		t.Errorf("expected ShutdownTimeout=15s, got %s", cfg.Server.ShutdownTimeout)
	}
	if !cfg.Cache.Enabled {
		t.Error("expected Cache.Enabled=true by default")
	}
	if len(cfg.Cache.RedisAddrs) != 1 || cfg.Cache.RedisAddrs[0] != "localhost:6380" {
		t.Errorf("unexpected RedisAddrs: %v", cfg.Cache.RedisAddrs)
	}
	if cfg.Cache.RedisTimeout != "5s" {
		t.Errorf("expected RedisTimeout=5s, got %s", cfg.Cache.RedisTimeout)
	}
	if !cfg.Audit.Enabled {
		t.Error("expected Audit.Enabled=true by default")
	}
	if cfg.Audit.GRPCUrl != "audit-service:50051" {
		t.Errorf("expected Audit.GRPCUrl=audit-service:50051, got %s", cfg.Audit.GRPCUrl)
	}
}

func TestLoad_FromYAML(t *testing.T) {
	dir := t.TempDir()
	yaml := `
server:
  service_name: from-yaml
  port: 9090
  logging_level: DEBUG
  shutdown_timeout: 30s
cache:
  enabled: false
  redis_addrs:
    - host1:6379
    - host2:6379
  redis_password: yamlpass
  redis_timeout: 10s
audit:
  enabled: true
  grpc_url: audit.test:80
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Load(dir)

	if cfg.Server.Port != 9090 {
		t.Errorf("expected Port=9090, got %d", cfg.Server.Port)
	}
	if cfg.Server.ServiceName != "from-yaml" {
		t.Errorf("expected ServiceName=from-yaml, got %s", cfg.Server.ServiceName)
	}
	if cfg.Server.LoggingLevel != "DEBUG" {
		t.Errorf("expected LoggingLevel=DEBUG, got %s", cfg.Server.LoggingLevel)
	}
	if cfg.Server.ShutdownTimeout != "30s" {
		t.Errorf("expected ShutdownTimeout=30s, got %s", cfg.Server.ShutdownTimeout)
	}
	if cfg.Cache.Enabled {
		t.Error("expected Cache.Enabled=false from YAML")
	}
	if len(cfg.Cache.RedisAddrs) != 2 || cfg.Cache.RedisAddrs[0] != "host1:6379" {
		t.Errorf("unexpected RedisAddrs: %v", cfg.Cache.RedisAddrs)
	}
	if cfg.Cache.RedisPassword != "yamlpass" {
		t.Errorf("expected RedisPassword=yamlpass, got %s", cfg.Cache.RedisPassword)
	}
	if cfg.Cache.RedisTimeout != "10s" {
		t.Errorf("expected RedisTimeout=10s, got %s", cfg.Cache.RedisTimeout)
	}
	if !cfg.Audit.Enabled {
		t.Error("expected Audit.Enabled=true from YAML")
	}
	if cfg.Audit.GRPCUrl != "audit.test:80" {
		t.Errorf("expected Audit.GRPCUrl=audit.test:80, got %s", cfg.Audit.GRPCUrl)
	}
}

func TestLoad_EnvOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	yaml := `
server:
  service_name: from-yaml
  port: 9090
cache:
  enabled: true
  redis_password: yamlpass
audit:
  enabled: true
  grpc_url: audit.yaml:80
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SERVER_PORT", "7777")
	t.Setenv("CACHE_REDIS_PASSWORD", "envpass")
	t.Setenv("AUDIT_GRPC_URL", "audit.env:80")

	cfg := Load(dir)

	// Env overrides YAML
	if cfg.Server.Port != 7777 {
		t.Errorf("expected env override Port=7777, got %d", cfg.Server.Port)
	}
	if cfg.Cache.RedisPassword != "envpass" {
		t.Errorf("expected env override RedisPassword=envpass, got %s", cfg.Cache.RedisPassword)
	}
	if cfg.Audit.GRPCUrl != "audit.env:80" {
		t.Errorf("expected env override Audit.GRPCUrl=audit.env:80, got %s", cfg.Audit.GRPCUrl)
	}
	// Non-overridden values come from YAML
	if cfg.Server.ServiceName != "from-yaml" {
		t.Errorf("expected ServiceName=from-yaml, got %s", cfg.Server.ServiceName)
	}
	if !cfg.Cache.Enabled {
		t.Error("expected Cache.Enabled=true from YAML")
	}
}
