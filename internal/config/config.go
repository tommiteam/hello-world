package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

const (
	confName = "config"
	confType = "yaml"
)

// Config is the root configuration, loaded from config.yaml and overridden by env vars.
//
// config.yaml is gitignored — it contains secrets. In Kubernetes it is mounted
// from a SealedSecret at /app/config.yaml. For local dev, create your own copy.
//
// YAML shape:
//
//	server:
//	  service_name: hello-world
//	  port: 8080
//	  logging_level: INFO
//	  shutdown_timeout: 15s
//	cache:
//	  redis_addrs:
//	    - redis-gateway.redis.svc.cluster.local:6379
//	  redis_password: ""
//	  redis_timeout: 5s
//	audit:
//	  enabled: true
//	  grpc_url: audit-service:50051
//
// Env-var override: flatten the YAML key path with "_", upper-case it.
//
//	SERVER_PORT=9090
//	SERVER_LOGGING_LEVEL=DEBUG
//	CACHE_REDIS_PASSWORD=secret
type Config struct {
	Server Server `mapstructure:"server"`
	Cache  Cache  `mapstructure:"cache"`
	Audit  Audit  `mapstructure:"audit"`
}

// Server holds HTTP server and general service settings.
type Server struct {
	ServiceName     string `mapstructure:"service_name"`
	Port            int    `mapstructure:"port"`
	LoggingLevel    string `mapstructure:"logging_level"`
	ShutdownTimeout string `mapstructure:"shutdown_timeout"` // e.g. "15s"
}

// Cache holds Redis connection settings.
type Cache struct {
	Enabled       bool     `mapstructure:"enabled"` // If false, skip Redis connection
	RedisAddrs    []string `mapstructure:"redis_addrs"`
	RedisPassword string   `mapstructure:"redis_password"`
	RedisTimeout  string   `mapstructure:"redis_timeout"` // e.g. "5s"
}

// Audit holds audit service connection settings.
type Audit struct {
	Enabled bool   `mapstructure:"enabled"`  // If false, skip audit client
	GRPCUrl string `mapstructure:"grpc_url"` // e.g. "audit.apps.svc.cluster.local:80"
}

// Load reads config.yaml from the given search paths (defaults to ".") and
// applies env-var overrides. Missing config file is not an error — built-in
// defaults are used instead (suitable for CI / unit tests).
// Panics on malformed config file or unmarshal errors (same convention as trala).
func Load(paths ...string) Config {
	v := viper.New()
	v.SetConfigName(confName)
	v.SetConfigType(confType)
	if len(paths) == 0 {
		paths = []string{"."}
	}
	for _, p := range paths {
		v.AddConfigPath(p)
	}
	// Built-in defaults — used when no config.yaml is found (e.g. CI, unit tests).
	v.SetDefault("server.service_name", "hello-world")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.logging_level", "INFO")
	v.SetDefault("server.shutdown_timeout", "15s")
	v.SetDefault("cache.enabled", true)
	v.SetDefault("cache.redis_addrs", []string{"localhost:6380"})
	v.SetDefault("cache.redis_password", "")
	v.SetDefault("cache.redis_timeout", "5s")
	v.SetDefault("audit.enabled", true)
	v.SetDefault("audit.grpc_url", "audit-service:50051")
	// Read file — tolerate "not found", panic on parse errors.
	if err := v.ReadInConfig(); err != nil {
		if _, notFound := err.(viper.ConfigFileNotFoundError); !notFound {
			panic(fmt.Errorf("config: read error: %w", err))
		}
	}
	// Env overrides: SERVER_PORT, CACHE_REDIS_PASSWORD, etc.
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		panic(fmt.Errorf("config: unmarshal error: %w", err))
	}
	return cfg
}
