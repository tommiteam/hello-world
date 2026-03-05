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
		"SERVER_SHUTDOWN_TIMEOUT", "CACHE_REDIS_ADDRS", "CACHE_REDIS_PASSWORD",
		"CACHE_REDIS_TIMEOUT",
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
	if len(cfg.Cache.RedisAddrs) != 1 || cfg.Cache.RedisAddrs[0] != "redis-gateway.redis.svc.cluster.local:6379" {
		t.Errorf("unexpected RedisAddrs: %v", cfg.Cache.RedisAddrs)
	}
	if cfg.Cache.RedisTimeout != "5s" {
		t.Errorf("expected RedisTimeout=5s, got %s", cfg.Cache.RedisTimeout)
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
  redis_addrs:
    - host1:6379
    - host2:6379
  redis_password: yamlpass
  redis_timeout: 10s
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
	if len(cfg.Cache.RedisAddrs) != 2 || cfg.Cache.RedisAddrs[0] != "host1:6379" {
		t.Errorf("unexpected RedisAddrs: %v", cfg.Cache.RedisAddrs)
	}
	if cfg.Cache.RedisPassword != "yamlpass" {
		t.Errorf("expected RedisPassword=yamlpass, got %s", cfg.Cache.RedisPassword)
	}
	if cfg.Cache.RedisTimeout != "10s" {
		t.Errorf("expected RedisTimeout=10s, got %s", cfg.Cache.RedisTimeout)
	}
}

func TestLoad_EnvOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	yaml := `
server:
  service_name: from-yaml
  port: 9090
cache:
  redis_password: yamlpass
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SERVER_PORT", "7777")
	t.Setenv("CACHE_REDIS_PASSWORD", "envpass")

	cfg := Load(dir)

	// Env overrides YAML
	if cfg.Server.Port != 7777 {
		t.Errorf("expected env override Port=7777, got %d", cfg.Server.Port)
	}
	if cfg.Cache.RedisPassword != "envpass" {
		t.Errorf("expected env override RedisPassword=envpass, got %s", cfg.Cache.RedisPassword)
	}
	// Non-overridden values come from YAML
	if cfg.Server.ServiceName != "from-yaml" {
		t.Errorf("expected ServiceName=from-yaml, got %s", cfg.Server.ServiceName)
	}
}
