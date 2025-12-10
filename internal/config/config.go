package config

import (
	"fmt"
	"os"
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
	Type string `mapstructure:"type"` // simplified or traditional
	Path string `mapstructure:"path"`
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
	MaxResults      int  `mapstructure:"max_results"`
	DefaultPageSize int  `mapstructure:"default_page_size"`
	EnablePinyin    bool `mapstructure:"enable_pinyin"`
	EnableFuzzy     bool `mapstructure:"enable_fuzzy"`
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

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "release")
	v.SetDefault("database.type", "simplified")
	v.SetDefault("database.path", "poetry-simplified.db")
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
	v.SetDefault("search.enable_pinyin", true)
	v.SetDefault("search.enable_fuzzy", true)
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

	// Database
	if dbMode := os.Getenv("DATABASE_MODE"); dbMode != "" {
		v.Set("database.type", dbMode)
		// Update path based on mode (for simplified/traditional only)
		if dbMode != "both" {
			v.Set("database.path", fmt.Sprintf("poetry-%s.db", dbMode))
		}
	}

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
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Server.Port)
	}

	if c.Server.Mode != "debug" && c.Server.Mode != "release" && c.Server.Mode != "test" {
		return fmt.Errorf("invalid server mode: %s (must be 'debug', 'release', or 'test')", c.Server.Mode)
	}

	if c.Database.Type != "simplified" && c.Database.Type != "traditional" && c.Database.Type != "both" {
		return fmt.Errorf("invalid database mode: %s (must be 'simplified', 'traditional', or 'both')", c.Database.Type)
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
