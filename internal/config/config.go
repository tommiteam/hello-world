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
// YAML structure:
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
//
// Every key can be overridden by an environment variable using the
// flattened, upper-cased path with "_" separators. For example:
//
//	SERVER_PORT=9090  SERVER_LOGGING_LEVEL=DEBUG  CACHE_REDIS_PASSWORD=secret
type Config struct {
	Server Server `mapstructure:"server"`
	Cache  Cache  `mapstructure:"cache"`
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
	RedisAddrs    []string `mapstructure:"redis_addrs"`
	RedisPassword string   `mapstructure:"redis_password"`
	RedisTimeout  string   `mapstructure:"redis_timeout"` // e.g. "5s"
}

// Load reads config.yaml from the given paths (or ".") and applies env overrides.
// It panics on fatal config errors (same convention as trala).
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

	// --- defaults (used when no config.yaml is present) ---
	v.SetDefault("server.service_name", "hello-world")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.logging_level", "INFO")
	v.SetDefault("server.shutdown_timeout", "15s")

	v.SetDefault("cache.redis_addrs", []string{"localhost:6380"})
	v.SetDefault("cache.redis_password", "")
	v.SetDefault("cache.redis_timeout", "5s")

	// --- read file (optional — works without one) ---
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			panic(fmt.Errorf("config: read error: %w", err))
		}
		// no file found → defaults + env only, which is fine
	}

	// --- env override (SERVER_PORT, CACHE_REDIS_PASSWORD, etc.) ---
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		panic(fmt.Errorf("config: unmarshal error: %w", err))
	}
	return cfg
}
