package config

import (
	"fmt"
	"net/netip"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

// Config 是应用的全部配置。
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	GraphQL   GraphQLConfig   `mapstructure:"graphql"`
	Search    SearchConfig    `mapstructure:"search"`
}

// ServerConfig 是服务端配置。
type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`

	// TrustedProxies 是允许设置 X-Forwarded-For 等转发头的代理地址（IP 或 CIDR）。
	// 只有来自这些地址的请求，其转发头才会被用来确定客户端 IP；
	// 其余请求一律以 TCP 对端地址为准，否则任何人改一下请求头就能绕过按 IP 限流。
	TrustedProxies []string `mapstructure:"trusted_proxies"`
}

// defaultTrustedProxies 只信任回环与私有网段：覆盖同机反代、Docker 网桥等常见部署，
// 又不会让直接来自公网的请求伪造客户端 IP。
// 若前面是公网上的 CDN（如直接接入 Cloudflare），需通过配置显式填写其地址段。
var defaultTrustedProxies = []string{
	"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
	"::1/128", "fc00::/7",
}

// DatabaseConfig 是数据库配置。
type DatabaseConfig struct {
	Path         string `mapstructure:"path"`
	MaxOpenConns int    `mapstructure:"max_open_conns"` // 最大连接数
	MaxIdleConns int    `mapstructure:"max_idle_conns"` // 最大空闲连接数
}

// DownloadConfig 是数据集下载相关的配置。
type DownloadConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	GithubRepo     string `mapstructure:"github_repo"`
	ReleaseVersion string `mapstructure:"release_version"`
}

// RateLimitConfig 是限流配置。
type RateLimitConfig struct {
	Enabled           bool    `mapstructure:"enabled"`
	RequestsPerSecond float64 `mapstructure:"requests_per_second"`
	Burst             int     `mapstructure:"burst"`
}

// GraphQLConfig 是 GraphQL 相关配置。
type GraphQLConfig struct {
	Playground    bool `mapstructure:"playground"`
	Introspection bool `mapstructure:"introspection"`
	// ComplexityLimit 是单个查询允许的最大复杂度，分页字段的开销按 pageSize 倍数计入，
	// 用来挡住层层嵌套、一次请求放大成海量查询的写法。
	ComplexityLimit int `mapstructure:"complexity_limit"`
}

// SearchConfig 是搜索相关配置。
type SearchConfig struct {
	MaxResults      int `mapstructure:"max_results"`
	DefaultPageSize int `mapstructure:"default_page_size"`
}

// Load 从配置文件与环境变量中加载配置。
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// 设置默认值
	setDefaults(v)

	// 指定了配置文件则读取
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// 环境变量优先级更高，覆盖前面的取值
	bindEnvVars(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 未配置连接池大小时按 CPU 核数自动推算
	cfg.applyConnectionPoolDefaults()

	// 校验配置
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "release")
	v.SetDefault("server.trusted_proxies", defaultTrustedProxies)
	v.SetDefault("download.enabled", true)
	v.SetDefault("download.release_version", "latest")
	v.SetDefault("rate_limit.enabled", true)
	v.SetDefault("rate_limit.requests_per_second", 10.0)
	v.SetDefault("rate_limit.burst", 20)
	v.SetDefault("rate_limit.by_ip", true)
	v.SetDefault("graphql.playground", false)
	v.SetDefault("graphql.introspection", true)
	v.SetDefault("graphql.complexity_limit", 5000)
	v.SetDefault("search.max_results", 1000)
	v.SetDefault("search.default_page_size", 20)
	// 数据库连接池，0 表示自动推算（在 Load 中依据 runtime.NumCPU 确定）
	v.SetDefault("database.max_open_conns", 0)
	v.SetDefault("database.max_idle_conns", 0)
}

func bindEnvVars(v *viper.Viper) {
	// 服务端
	if port := os.Getenv("PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			v.Set("server.port", p)
		}
	}
	if mode := os.Getenv("GIN_MODE"); mode != "" {
		v.Set("server.mode", mode)
	}
	// 以逗号分隔；设为 none 表示不信任任何代理
	if proxies, ok := os.LookupEnv("TRUSTED_PROXIES"); ok {
		v.Set("server.trusted_proxies", parseProxyList(proxies))
	}

	// 数据目录写死，与 docker-compose 中挂载的卷保持一致
	dataDir := "data"

	// 统一使用 poetry.db，其中同时包含简体与繁体两套表；
	// 具体查哪套由 API 请求中的 lang 参数决定
	v.Set("database.path", fmt.Sprintf("%s/poetry.db", dataDir))

	// 限流
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

	// 数据库连接池
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

// Validate 校验配置的合法性。
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

	for _, proxy := range c.Server.TrustedProxies {
		if !isIPOrPrefix(proxy) {
			return fmt.Errorf("invalid trusted proxy %q: must be an IP or CIDR", proxy)
		}
	}

	if c.GraphQL.ComplexityLimit <= 0 {
		return fmt.Errorf("graphql complexity_limit must be positive")
	}

	if c.RateLimit.RequestsPerSecond <= 0 {
		return fmt.Errorf("rate limit requests_per_second must be positive")
	}

	if c.RateLimit.Burst <= 0 {
		return fmt.Errorf("rate limit burst must be positive")
	}

	return nil
}

// applyConnectionPoolDefaults 依据 CPU 核数为连接池设置合理的默认值。
func (c *Config) applyConnectionPoolDefaults() {
	numCPU := runtime.NumCPU()

	// max_open_conns 未配置（为 0 或负数）时自动推算
	if c.Database.MaxOpenConns <= 0 {
		// 按核数自适应：
		//   - 多核（>4）：直接取核数，并行度已足够
		//   - 少核（≤4）：取核数的两倍，以更好地利用 I/O 等待时间
		//   - 统一以 50 封顶，避免连接数过多
		if numCPU > 4 {
			c.Database.MaxOpenConns = min(numCPU, 50)
		} else {
			c.Database.MaxOpenConns = min(numCPU*2, 50)
		}
	}

	// max_idle_conns 未配置时自动推算
	if c.Database.MaxIdleConns <= 0 {
		// 空闲连接数取最大连接数的一半左右
		c.Database.MaxIdleConns = max(c.Database.MaxOpenConns/2, 1)
	}
}

// parseProxyList 解析逗号分隔的代理列表，"none" 或空串表示不信任任何代理。
func parseProxyList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "none") {
		return []string{}
	}

	var proxies []string
	for item := range strings.SplitSeq(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			proxies = append(proxies, item)
		}
	}
	return proxies
}

func isIPOrPrefix(s string) bool {
	if _, err := netip.ParseAddr(s); err == nil {
		return true
	}
	_, err := netip.ParsePrefix(s)
	return err == nil
}
