package config

import (
	"fmt"
	"os"
	"runtime"
	"strconv"

	"github.com/spf13/viper"
)

// Config holds all configuration for the application
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	GraphQL   GraphQLConfig   `mapstructure:"graphql"`
	Search    SearchConfig    `mapstructure:"search"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Path         string `mapstructure:"path"`
	MaxOpenConns int    `mapstructure:"max_open_conns"` // Maximum number of open connections
	MaxIdleConns int    `mapstructure:"max_idle_conns"` // Maximum number of idle connections
}

type DownloadConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	GithubRepo     string `mapstructure:"github_repo"`
	ReleaseVersion string `mapstructure:"release_version"`
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled           bool    `mapstructure:"enabled"`
	RequestsPerSecond float64 `mapstructure:"requests_per_second"`
	Burst             int     `mapstructure:"burst"`
}

// GraphQLConfig holds GraphQL configuration
type GraphQLConfig struct {
	Playground bool `mapstructure:"playground"`
}

// SearchConfig holds search configuration
type SearchConfig struct {
	MaxResults      int `mapstructure:"max_results"`
	DefaultPageSize int `mapstructure:"default_page_size"`
}

// Load loads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Read config file if provided
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Override with environment variables
	bindEnvVars(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Auto-detect connection pool size based on CPU cores if not configured
	cfg.applyConnectionPoolDefaults()

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "release")
	v.SetDefault("download.enabled", true)
	v.SetDefault("download.release_version", "latest")
	v.SetDefault("rate_limit.enabled", true)
	v.SetDefault("rate_limit.requests_per_second", 10.0)
	v.SetDefault("rate_limit.burst", 20)
	v.SetDefault("rate_limit.by_ip", true)
	v.SetDefault("graphql.playground", false)
	v.SetDefault("graphql.introspection", true)
	v.SetDefault("graphql.complexity_limit", 1000)
	v.SetDefault("search.max_results", 1000)
	v.SetDefault("search.default_page_size", 20)
	// Database connection pool - auto-detect based on CPU cores
	// 0 means auto-detect (will be set to runtime.NumCPU() in Load())
	v.SetDefault("database.max_open_conns", 0)
	v.SetDefault("database.max_idle_conns", 0)
}

func bindEnvVars(v *viper.Viper) {
	// Server
	if port := os.Getenv("PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			v.Set("server.port", p)
		}
	}
	if mode := os.Getenv("GIN_MODE"); mode != "" {
		v.Set("server.mode", mode)
	}

	// Hardcoded data directory (matches docker-compose volume mount)
	dataDir := "data"

	// Database - use unified poetry.db (contains both simplified and traditional tables)
	// The lang parameter in API requests determines which tables to query
	v.Set("database.path", fmt.Sprintf("%s/poetry.db", dataDir))

	// Rate Limit
	if enabled := os.Getenv("RATE_LIMIT_ENABLED"); enabled != "" {
		v.Set("rate_limit.enabled", enabled == "true")
	}
	if rps := os.Getenv("RATE_LIMIT_RPS"); rps != "" {
		if r, err := strconv.ParseFloat(rps, 64); err == nil {
			v.Set("rate_limit.requests_per_second", r)
		}
	}
	if burst := os.Getenv("RATE_LIMIT_BURST"); burst != "" {
		if b, err := strconv.Atoi(burst); err == nil {
			v.Set("rate_limit.burst", b)
		}
	}

	// Database connection pool
	if maxOpen := os.Getenv("DB_MAX_OPEN_CONNS"); maxOpen != "" {
		if m, err := strconv.Atoi(maxOpen); err == nil {
			v.Set("database.max_open_conns", m)
		}
	}
	if maxIdle := os.Getenv("DB_MAX_IDLE_CONNS"); maxIdle != "" {
		if m, err := strconv.Atoi(maxIdle); err == nil {
			v.Set("database.max_idle_conns", m)
		}
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Server.Port)
	}

	if c.Server.Mode != "debug" && c.Server.Mode != "release" && c.Server.Mode != "test" {
		return fmt.Errorf("invalid server mode: %s (must be 'debug', 'release', or 'test')", c.Server.Mode)
	}

	if c.Database.Path == "" {
		return fmt.Errorf("database path cannot be empty")
	}

	if c.RateLimit.RequestsPerSecond <= 0 {
		return fmt.Errorf("rate limit requests_per_second must be positive")
	}

	if c.RateLimit.Burst <= 0 {
		return fmt.Errorf("rate limit burst must be positive")
	}

	return nil
}

// applyConnectionPoolDefaults sets intelligent defaults for connection pool based on CPU cores
func (c *Config) applyConnectionPoolDefaults() {
	numCPU := runtime.NumCPU()

	// Auto-detect max_open_conns if not configured (0 or negative)
	if c.Database.MaxOpenConns <= 0 {
		// Adaptive strategy based on CPU count:
		// - Multi-core (>4): Use NumCPU directly (sufficient parallelism)
		// - Few cores (≤4): Use NumCPU*2 to better utilize I/O wait time
		// - Cap at 50 to prevent excessive connections
		if numCPU > 4 {
			c.Database.MaxOpenConns = min(numCPU, 50)
		} else {
			c.Database.MaxOpenConns = min(numCPU*2, 50)
		}
	}

	// Auto-detect max_idle_conns if not configured
	if c.Database.MaxIdleConns <= 0 {
		// Idle connections should be about half of max open connections
		c.Database.MaxIdleConns = max(c.Database.MaxOpenConns/2, 1)
	}
}
